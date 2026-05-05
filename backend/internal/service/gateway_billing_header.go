package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ccVersionInBillingRe matches the semver part of cc_version (X.Y.Z), preserving
// the trailing message-derived suffix (e.g. ".c02") if present.
var ccVersionInBillingRe = regexp.MustCompile(`cc_version=\d+\.\d+\.\d+`)

// ccVersionFullRe 匹配 cc_version 的完整三段式版本号加三段十六进制指纹后缀
// 例如 cc_version=2.1.92.a3f 中，捕获组 1 为 2.1.92，捕获组 2 为 a3f
var ccVersionFullRe = regexp.MustCompile(`cc_version=(\d+\.\d+\.\d+)\.([a-f0-9]{3})`)

// cchPlaceholderRe matches the cch=00000 placeholder in billing header text,
// scoped to x-anthropic-billing-header to avoid touching user content.
var cchPlaceholderRe = regexp.MustCompile(`(x-anthropic-billing-header:[^"]*?\bcch=)(00000)(;)`)

const cchSeed uint64 = 0x6E52736AC806831E

// syncBillingHeaderVersion rewrites cc_version in x-anthropic-billing-header
// system text blocks to match the version extracted from userAgent.
// Only touches system array blocks whose text starts with "x-anthropic-billing-header".
func syncBillingHeaderVersion(body []byte, userAgent string) []byte {
	version := ExtractCLIVersion(userAgent)
	if version == "" {
		return body
	}

	systemResult := gjson.GetBytes(body, "system")
	if !systemResult.Exists() || !systemResult.IsArray() {
		return body
	}

	replacement := "cc_version=" + version
	idx := 0
	systemResult.ForEach(func(_, item gjson.Result) bool {
		text := item.Get("text")
		if text.Exists() && text.Type == gjson.String &&
			strings.HasPrefix(text.String(), "x-anthropic-billing-header") {
			newText := ccVersionInBillingRe.ReplaceAllString(text.String(), replacement)
			if newText != text.String() {
				if updated, err := sjson.SetBytes(body, fmt.Sprintf("system.%d.text", idx), newText); err == nil {
					body = updated
				}
			}
		}
		idx++
		return true
	})

	return body
}

// signBillingHeaderCCH computes the xxHash64-based CCH signature for the request
// body and replaces the cch=00000 placeholder with the computed 5-hex-char hash.
// The body must contain the placeholder when this function is called.
func signBillingHeaderCCH(body []byte) []byte {
	if !cchPlaceholderRe.Match(body) {
		return body
	}
	cch := fmt.Sprintf("%05x", xxHash64Seeded(body, cchSeed)&0xFFFFF)
	return cchPlaceholderRe.ReplaceAll(body, []byte("${1}"+cch+"${3}"))
}

// xxHash64Seeded computes xxHash64 of data with a custom seed.
func xxHash64Seeded(data []byte, seed uint64) uint64 {
	d := xxhash.NewWithSeed(seed)
	_, _ = d.Write(data)
	return d.Sum64()
}

// forceBillingCCVersionFingerprint 在遥测隐私保护开启时，将计费头中 cc_version
// 的三段十六进制指纹后缀替换为账号级确定性值。
//
// 背景：原始的 computeClaudeCodeFingerprint 从首条用户消息的三个固定位置取字符
// 计算指纹，不同用户的不同消息会产生不同的 fp。同一上游账号长时间呈现大量不同的
// cc_version fp 会被 Anthropic 视为异常信号，与"不要让上游觉得很多人在用"冲突。
//
// 本函数使用账号 ID + CLI 版本号派生一个固定的三段十六进制指纹，确保同一账号
// 始终以相同的 cc_version 指纹出现在上游，消除内容指纹维度的用户分化。
//
// 替换范围限定在 system 数组中 text 以 "x-anthropic-billing-header" 开头的块，
// 与 syncBillingHeaderVersion 保持一致，不会误改用户消息或工具参数中的类似文本。
//
// 参数:
//   - body: 请求正文（system 数组中应包含 billing attribution block）
//   - account: 当前账号，为 nil 或遥测隐私未开启时原样返回 body
//
// 返回:
//   - []byte: 修改后的请求正文（cc_version fp 已替换为确定性值）
func forceBillingCCVersionFingerprint(body []byte, account *Account) []byte {
	if account == nil || !account.IsTelemetryPrivacyEnabled() {
		return body
	}

	systemResult := gjson.GetBytes(body, "system")
	if !systemResult.Exists() || !systemResult.IsArray() {
		return body
	}

	// 使用账号 ID 派生确定性 fp，保证同一账号始终相同
	fpInput := fmt.Sprintf("telemetry_privacy_fp:%d:%s", account.ID, claude.CLICurrentVersion)
	sum := sha256.Sum256([]byte(fpInput))
	fp := hex.EncodeToString(sum[:])[:3]
	replacement := "cc_version=${1}." + fp

	idx := 0
	modified := false
	systemResult.ForEach(func(_, item gjson.Result) bool {
		text := item.Get("text")
		if text.Exists() && text.Type == gjson.String &&
			strings.HasPrefix(text.String(), "x-anthropic-billing-header") {
			newText := ccVersionFullRe.ReplaceAllString(text.String(), replacement)
			if newText != text.String() {
				if updated, err := sjson.SetBytes(body, fmt.Sprintf("system.%d.text", idx), newText); err == nil {
					body = updated
					modified = true
				}
			}
		}
		idx++
		return true
	})

	// 未找到 billing header 块时原样返回，不额外新增块
	if !modified {
		return body
	}
	return body
}
