package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// defaultPrivacyHMACKey 是遥测隐私 HMAC-SHA256 的默认后备密钥（32 字节）。
// 仅在未设置环境变量且数据库中也无自定义密钥时使用。
// 注意：此默认值提交至仓库，仅用于开发/测试环境。
var defaultPrivacyHMACKey = [32]byte{
	0x9e, 0x3c, 0x7a, 0x1f, 0x55, 0xe8, 0x2b, 0x4d,
	0xa6, 0x91, 0x0d, 0x33, 0x7e, 0x5c, 0x88, 0x12,
	0xbf, 0x60, 0x4a, 0x19, 0xd7, 0x2e, 0x95, 0x3f,
	0x08, 0x6c, 0x41, 0xab, 0x1d, 0x57, 0xe9, 0x26,
}

// privacyHMACKey 在首次调用 getPrivacyHMACKey() 时通过 sync.Once 初始化。
// 初始化优先级：环境变量 → 数据库设置（由 InitPrivacyHMACKeyFromDB 注入）→ 默认后备密钥。
// 格式：32 字节原始密钥。
var (
	privacyHMACKey     []byte
	privacyHMACKeyOnce sync.Once
	dbHMACKeyHex       string   // 数据库设置的原始十六进制字符串，由 InitPrivacyHMACKeyFromDB 设置
	dbHMACKeyMu        sync.Mutex // 保护 dbHMACKeyHex 的并发访问
)

// SettingKeyTelemetryPrivacyHMACKey 是遥测隐私 HMAC 密钥在 settings 表中的 key。
const SettingKeyTelemetryPrivacyHMACKey = "telemetry_privacy_hmac_key"

// InitPrivacyHMACKeyFromDB 从数据库设置中加载遥测隐私 HMAC 密钥。
// 应在应用启动阶段、数据库连接就绪后调用一次。
// 优先级：环境变量 TELEMETRY_PRIVACY_HMAC_KEY > DB 设置 > 默认后备密钥。
// 若环境变量已设置，则忽略 DB 值（环境变量具有最高优先级）。
func InitPrivacyHMACKeyFromDB(ctx context.Context, settingRepo SettingRepository) {
	dbHMACKeyMu.Lock()
	defer dbHMACKeyMu.Unlock()

	if settingRepo != nil {
		if val, err := settingRepo.GetValue(ctx, SettingKeyTelemetryPrivacyHMACKey); err == nil && val != "" {
			dbHMACKeyHex = val
		}
	}
	// 触发 sync.Once 初始化（若尚未初始化）
	_ = getPrivacyHMACKey()
}

// SetPrivacyHMACKeyFromAdmin 由管理后台调用，更新内存中的 HMAC 密钥。
// 同时持久化到数据库 settings 表。
// 注意：密钥变更后，后续请求将使用新密钥派生 device_id/UUID，
// 可能导致同一账号会话中身份标识发生变化。
// 建议在低流量时段操作，或提示管理员重启服务以确保一致性。
func SetPrivacyHMACKeyFromAdmin(ctx context.Context, settingRepo SettingRepository, hexKey string) error {
	if hexKey == "" {
		return fmt.Errorf("HMAC 密钥不能为空")
	}
	keyBytes, err := hex.DecodeString(hexKey)
	if err != nil || len(keyBytes) != 32 {
		return fmt.Errorf("HMAC 密钥必须为 64 位十六进制字符串（32 字节）")
	}

	// 持久化到数据库
	if settingRepo != nil {
		if err := settingRepo.Set(ctx, SettingKeyTelemetryPrivacyHMACKey, hexKey); err != nil {
			return fmt.Errorf("保存 HMAC 密钥到数据库失败: %w", err)
		}
	}

	// 更新内存中的密钥
	dbHMACKeyMu.Lock()
	dbHMACKeyHex = hexKey
	dbHMACKeyMu.Unlock()

	// 重置 sync.Once 以使用新密钥（Go 1.21+ 支持 Once.Reset）
	// 注意：sync.Once 的语义限制 — Reset 后重新初始化的正确性由调用方保证。
	privacyHMACKeyOnce = sync.Once{}
	privacyHMACKey = nil

	slog.Info("telemetry_privacy_hmac_key_updated_from_admin")
	return nil
}

// getPrivacyHMACKey 返回当前的 HMAC 密钥（懒初始化，线程安全）。
// 初始化优先级：环境变量 → 数据库设置（由 InitPrivacyHMACKeyFromDB 注入）→ 默认后备密钥。
func getPrivacyHMACKey() []byte {
	privacyHMACKeyOnce.Do(func() {
		// 1. 环境变量（最高优先级）
		if hexKey := os.Getenv("TELEMETRY_PRIVACY_HMAC_KEY"); hexKey != "" {
			key, err := hex.DecodeString(hexKey)
			if err != nil || len(key) != 32 {
				slog.Error("telemetry_privacy_invalid_hmac_key_env", "error", err)
				panic("TELEMETRY_PRIVACY_HMAC_KEY must be a 64-char hex string (32 bytes)")
			}
			privacyHMACKey = key
			slog.Info("telemetry_privacy_hmac_key_loaded_from_env")
			return
		}

		// 2. 数据库设置（由 InitPrivacyHMACKeyFromDB 预先注入）
		dbHMACKeyMu.Lock()
		dbKey := dbHMACKeyHex
		dbHMACKeyMu.Unlock()
		if dbKey != "" {
			key, err := hex.DecodeString(dbKey)
			if err == nil && len(key) == 32 {
				privacyHMACKey = key
				slog.Info("telemetry_privacy_hmac_key_loaded_from_db")
				return
			}
			slog.Warn("telemetry_privacy_hmac_key_invalid_db", "error", err)
		}

		// 3. 默认后备密钥
		privacyHMACKey = defaultPrivacyHMACKey[:]
		slog.Warn("telemetry_privacy_hmac_key_using_default",
			"warning", "默认 HMAC 密钥为公开源码常量，生产部署请在系统设置中配置自定义密钥或设置 TELEMETRY_PRIVACY_HMAC_KEY 环境变量")
	})
	return privacyHMACKey
}

// hmacPrivacy 对输入数据执行 HMAC-SHA256 计算，返回原始字节。
// 使用 getPrivacyHMACKey() 获取的密钥，同一密钥下同一输入始终产生相同输出。
func hmacPrivacy(data string) []byte {
	mac := hmac.New(sha256.New, getPrivacyHMACKey())
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// globalEventLoggingDrops 全局遥测日志丢弃计数器。
// /api/event_logging/batch 路径由 common.go 无条件静默丢弃（无需认证），
// 因此无法归属到具体账号。此计数器用于记录平台级别的总丢弃次数。
var globalEventLoggingDrops atomic.Int64

// IncrementGlobalEventLoggingDrops 递增全局遥测日志丢弃计数。
// 由 common.go 中的 /api/event_logging/batch 处理器调用。
func IncrementGlobalEventLoggingDrops() int64 {
	return globalEventLoggingDrops.Add(1)
}

// GetGlobalEventLoggingDrops 返回全局遥测日志丢弃计数的当前快照。
func GetGlobalEventLoggingDrops() int64 {
	return globalEventLoggingDrops.Load()
}

var (
	rePlatform   = regexp.MustCompile(`(?im)^Platform:\s*[^\n]+`)
	reShell      = regexp.MustCompile(`(?im)^Shell:\s*[^\n]+`)
	reOSVersion  = regexp.MustCompile(`(?im)^OS Version:\s*[^\n]+`)
	reWorkingDir = regexp.MustCompile(`(?mi)^(Primary )?Working directory:\s*[^\n]+`)
	reHomeDir    = regexp.MustCompile(`(?mi)/(?:Users|home)/[^/\s]+/`)
)

// Telemetry privacy action counters (atomic, per-account, for observability).
type accountPrivacyCounters struct {
	droppedTelemetry atomic.Int64
	strippedBody     atomic.Int64
	strippedHeaders  atomic.Int64
}

var accountPrivacyStats sync.Map // map[int64]*accountPrivacyCounters (keyed by account ID)

func getAccountCounters(accountID int64) *accountPrivacyCounters {
	if v, ok := accountPrivacyStats.Load(accountID); ok {
		return v.(*accountPrivacyCounters)
	}
	c := &accountPrivacyCounters{}
	actual, _ := accountPrivacyStats.LoadOrStore(accountID, c)
	return actual.(*accountPrivacyCounters)
}

// TelemetryPrivacyStats 遥测隐私操作计数器的快照。
// GlobalEventLoggingDrops 为全平台级别（/api/event_logging/batch 无认证拦截），
// 其余字段为每账号级别。
type TelemetryPrivacyStats struct {
	DroppedTelemetry        int64 `json:"dropped_telemetry"`
	StrippedBody            int64 `json:"stripped_body"`
	StrippedHeaders         int64 `json:"stripped_headers"`
	GlobalEventLoggingDrops int64 `json:"global_event_logging_drops"`
}

// GetTelemetryPrivacyStats 返回指定账号的遥测隐私计数器快照。
// GlobalEventLoggingDrops 为全平台级别，同一时刻对所有账号返回相同值。
func GetTelemetryPrivacyStats(accountID int64) TelemetryPrivacyStats {
	c := getAccountCounters(accountID)
	return TelemetryPrivacyStats{
		DroppedTelemetry:        c.droppedTelemetry.Load(),
		StrippedBody:            c.strippedBody.Load(),
		StrippedHeaders:         c.strippedHeaders.Load(),
		GlobalEventLoggingDrops: globalEventLoggingDrops.Load(),
	}
}

// GetAllTelemetryPrivacyStats returns a snapshot of privacy counters for all accounts.
func GetAllTelemetryPrivacyStats() map[int64]TelemetryPrivacyStats {
	g := globalEventLoggingDrops.Load()
	result := make(map[int64]TelemetryPrivacyStats)
	accountPrivacyStats.Range(func(key, value any) bool {
		id := key.(int64)
		c := value.(*accountPrivacyCounters)
		result[id] = TelemetryPrivacyStats{
			DroppedTelemetry:        c.droppedTelemetry.Load(),
			StrippedBody:            c.strippedBody.Load(),
			StrippedHeaders:         c.strippedHeaders.Load(),
			GlobalEventLoggingDrops: g,
		}
		return true
	})
	return result
}

// IsAnthropicTelemetryPrivacyEnabled returns true if telemetry privacy is enabled for the account.
// Only applies to Anthropic OAuth/SetupToken accounts with telemetry_privacy=true in Extra.
func IsAnthropicTelemetryPrivacyEnabled(account *Account) bool {
	if account == nil {
		return false
	}
	if !account.IsAnthropicOAuthOrSetupToken() {
		return false
	}
	if account.Extra == nil {
		return false
	}
	raw, ok := account.Extra[domain.ExtraKeyTelemetryPrivacy]
	if !ok {
		return false
	}
	enabled, _ := raw.(bool)
	return enabled
}

// derivePrivacyDeviceID 基于账号 ID 确定性派生 64 位十六进制设备 ID。
// 每个账号获得唯一且一致的设备 ID，使不同账号呈现为不同用户，
// 避免全零值被 Anthropic 风控检测为代理指纹。
// 使用 HMAC-SHA256 配合编译时嵌入密钥确保不可逆，
// 即使攻击者获取源码也无法枚举 accountID → device_id 映射。
func derivePrivacyDeviceID(accountID int64) string {
	data := fmt.Sprintf("device-%d", accountID)
	return hex.EncodeToString(hmacPrivacy(data)) // 64 字符十六进制
}

// derivePrivacyUUID 基于账号 ID 和命名空间确定性派生 UUID v4 格式标识符。
// 同一 (accountID, namespace) 组合始终返回相同 UUID，不同账号或命名空间返回不同值。
// 使用 HMAC-SHA256 配合编译时嵌入密钥，不可逆，用于生成 account_uuid 和 session_id。
func derivePrivacyUUID(accountID int64, namespace string) string {
	data := fmt.Sprintf("%s-%d", namespace, accountID)
	b := hmacPrivacy(data)[:16]
	// 设置 UUID v4 版本位 (4) 和变体位 (10xx)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ShouldDropTelemetryEndpoint 判断请求是否应被静默丢弃（返回 200 而不转发上游）。
// 同时返回匹配的遥测类别。用于 TelemetryIntercept 拦截处理器和 Forward() 中的兜底检查。
func ShouldDropTelemetryEndpoint(path, host string) (drop bool, category string) {
	// 事件日志批处理（Channel B：Claude Code 主要遥测通道）
	if strings.Contains(path, "/api/event_logging") {
		return true, "event_logging"
	}
	// Claude Code Bug 报告/反馈端点 — 包含完整对话转录、错误信息和系统环境指纹
	// CC-Source 路径：POST /api/claude_cli_feedback
	if strings.Contains(path, "/api/claude_cli_feedback") {
		return true, "cli_feedback"
	}
	// OAuth CLI 端点 — API Key 创建（create_api_key）和角色查询（roles），携带 OAuth 令牌
	// CC-Source 路径：/api/oauth/claude_cli/create_api_key、/api/oauth/claude_cli/roles
	if strings.Contains(path, "/api/oauth/claude_cli") {
		return true, "oauth_cli"
	}
	// 上游代理中继（CCR 远程代理通道）
	if strings.Contains(path, "/upstreamproxy") {
		return true, "upstream_proxy"
	}
	// Grove 设置 API — 携带 OAuth 令牌和账户元数据
	// 匹配 /api/oauth/account 及其所有子路径（settings、grove_notice_viewed 等）
	if strings.Contains(path, "/api/oauth/account") {
		return true, "grove_settings"
	}
	// Grove 功能配置
	if strings.Contains(path, "/api/claude_code_grove") {
		return true, "grove_config"
	}
	// Claude Code 内部 API — BigQuery 指标、设置同步、团队记忆等
	// 匹配 /api/claude_code/metrics、/api/claude_code/user_settings、
	// /api/claude_code/team_memory、/api/claude_code/policy_limits 等
	if strings.Contains(path, "/api/claude_code/") {
		return true, "claude_code_internal"
	}
	// 会话转录分享（反馈/遥测）
	if strings.Contains(path, "/api/claude_code_shared_session_transcripts") {
		return true, "transcript_share"
	}
	// Claude Code 企鹅模式配置（非必要遥测）
	if strings.Contains(path, "/api/claude_code_penguin_mode") {
		return true, "penguin_mode"
	}
	// 主机级别拦截参考 cc-gateway 的 Clash 规则（纵深防御）。
	// 注意：Datadog 直连 datadoghq.com，不经 sub2api 代理，此拦截仅对
	// 误配置将 Datadog 流量路由至代理的场景起防御作用。彻底阻断需客户端
	// 设置 DISABLE_TELEMETRY=1 环境变量或在网络层屏蔽 *.datadoghq.com。
	lowerHost := strings.ToLower(host)
	if strings.Contains(lowerHost, "datadoghq.com") {
		return true, "datadog"
	}
	if strings.Contains(lowerHost, "storage.googleapis.com") {
		return true, "update_check"
	}
	return false, ""
}

// LogAccountTelemetryDrop logs a telemetry drop event and increments the per-account counter.
// Exported for use by the handler layer (TelemetryIntercept).
func LogAccountTelemetryDrop(account *Account, category, path, host string) {
	c := getAccountCounters(account.ID)
	n := c.droppedTelemetry.Add(1)
	slog.Info("telemetry_privacy_drop",
		"account_id", account.ID,
		"account_name", account.Name,
		"category", category,
		"path", path,
		"host", host,
		"account_drops", n,
	)
}

// logBodyStrip logs and counts body privacy stripping events.
func logBodyStrip(account *Account) {
	c := getAccountCounters(account.ID)
	n := c.strippedBody.Add(1)
	if n%100 == 1 {
		slog.Info("telemetry_privacy_body_strip",
			"account_id", account.ID,
			"account_name", account.Name,
			"account_stripped", n,
		)
	}
}

// logHeadersStrip logs and counts header stripping events.
func logHeadersStrip(account *Account) {
	c := getAccountCounters(account.ID)
	n := c.strippedHeaders.Add(1)
	if n%100 == 1 {
		slog.Info("telemetry_privacy_headers_strip",
			"account_id", account.ID,
			"account_name", account.Name,
			"account_stripped", n,
		)
	}
}

// RewriteTelemetryUserID 基于账号 ID 确定性重写 metadata.user_id 字段。
// 使用 accountID 派生每账号唯一且一致的 device_id、account_uuid、session_id，
// 替代原有的全零规范值。每个账号获得独立且稳定的身份标识，
// 避免数千用户共用同一全零 device_id 被 Anthropic 风控检测为代理指纹。
// 派生函数使用 SHA-256，不可逆，上游无法反推原始 accountID。
// accountID 参数由调用方（gateway_service.go 中的 buildUpstreamRequest）传入。
func RewriteTelemetryUserID(body []byte, accountID int64) []byte {
	if len(body) == 0 {
		return body
	}

	userID := gjson.GetBytes(body, "metadata.user_id").String()
	if userID == "" {
		return body
	}

	_ = ParseMetadataUserID(userID) // 验证格式 — 无论原始格式为何均覆盖

	// 基于账号 ID 确定性派生每账号唯一身份标识
	// device_id：64 字符十六进制（模拟 Claude Code 随机生成的设备 ID）
	// account_uuid：UUID v4 格式（模拟 OAuth 账户 UUID）
	// session_id：UUID v4 格式（模拟会话标识符，每账号一致性以保持单用户外观）
	canonical := FormatMetadataUserID(
		derivePrivacyDeviceID(accountID),
		derivePrivacyUUID(accountID, "account"),
		derivePrivacyUUID(accountID, "session"),
		"9.9.9",
	)

	modified, err := sjson.SetBytes(body, "metadata.user_id", canonical)
	if err != nil {
		return body
	}
	return modified
}

// StripTelemetryBillingHeader removes x-anthropic-billing-header blocks from the system array.
func StripTelemetryBillingHeader(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	system := gjson.GetBytes(body, "system")
	if !system.IsArray() {
		return body
	}

	filtered := make([]any, 0)
	for _, block := range system.Array() {
		text := block.Get("text").String()
		if strings.HasPrefix(strings.ToLower(text), "x-anthropic-billing-header:") {
			continue
		}
		filtered = append(filtered, block.Value())
	}

	if len(filtered) == len(system.Array()) {
		return body
	}

	modified, err := sjson.SetBytes(body, "system", filtered)
	if err != nil {
		return body
	}
	return modified
}

// StripTelemetryEnvInfo replaces environment-identifying information in system text blocks
// with canonical placeholders.
func StripTelemetryEnvInfo(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	system := gjson.GetBytes(body, "system")
	if !system.IsArray() {
		return body
	}

	modified := false
	for i, block := range system.Array() {
		text := block.Get("text").String()
		if text == "" {
			continue
		}
		// Skip billing-header blocks
		if strings.HasPrefix(strings.ToLower(text), "x-anthropic-billing-header:") {
			continue
		}

		newText := text
		newText = rePlatform.ReplaceAllString(newText, "Platform: unknown")
		newText = reShell.ReplaceAllString(newText, "Shell: unknown")
		newText = reOSVersion.ReplaceAllString(newText, "OS Version: unknown")
		newText = reWorkingDir.ReplaceAllString(newText, "Working directory: /workspace")
		newText = reHomeDir.ReplaceAllString(newText, "/home/user/")

		if newText != text {
			path := fmt.Sprintf("system.%d.text", i)
			var err error
			body, err = sjson.SetBytes(body, path, newText)
			if err != nil {
				continue
			}
			modified = true
		}
	}

	if !modified {
		return body
	}
	return body
}

// reSystemReminder 匹配 <system-reminder>...</system-reminder> 标签及其内容。
// Claude Code 将环境信息注入到用户消息中的 <system-reminder> 块内，
// 必须将其中的 Platform/Shell/OS Version/Working Directory/Home Dir 等字段替换为规范值。
var reSystemReminder = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)

// rewriteSystemReminderContent 对单段 text 内容中的 <system-reminder> 块执行环境信息替换。
// 返回替换后的文本；若未匹配到 <system-reminder> 标签则返回原文本。
func rewriteSystemReminderContent(text string) string {
	return reSystemReminder.ReplaceAllStringFunc(text, func(match string) string {
		rewritten := match
		rewritten = rePlatform.ReplaceAllString(rewritten, "Platform: unknown")
		rewritten = reShell.ReplaceAllString(rewritten, "Shell: unknown")
		rewritten = reOSVersion.ReplaceAllString(rewritten, "OS Version: unknown")
		rewritten = reWorkingDir.ReplaceAllString(rewritten, "Working directory: /workspace")
		rewritten = reHomeDir.ReplaceAllString(rewritten, "/home/user/")
		return rewritten
	})
}

// StripTelemetrySystemReminders 重写消息中 <system-reminder> 块内的环境识别信息。
// Claude Code 会在用户消息的 text content 块中嵌入 <system-reminder> 标签，
// 其中包含 Platform、Shell、OS Version、Working Directory、Home Dir 等环境指纹。
// 此函数遍历所有 messages，在 text 类型的 content 块中查找并替换这些标签的内容。
func StripTelemetrySystemReminders(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}

	modified := false
	for i, msg := range messages.Array() {
		content := msg.Get("content")

		// 处理 string 类型 content（Anthropic API 支持纯字符串消息）
		if content.Type == gjson.String {
			text := content.String()
			if text == "" {
				continue
			}
			newText := rewriteSystemReminderContent(text)
			if newText != text {
				path := fmt.Sprintf("messages.%d.content", i)
				var err error
				body, err = sjson.SetBytes(body, path, newText)
				if err != nil {
					continue
				}
				modified = true
			}
			continue
		}

		// 处理 array 类型 content（标准多 block 消息格式）
		if !content.IsArray() {
			continue
		}

		for j, block := range content.Array() {
			text := block.Get("text").String()
			if text == "" {
				continue
			}

			newText := rewriteSystemReminderContent(text)
			if newText != text {
				path := fmt.Sprintf("messages.%d.content.%d.text", i, j)
				var err error
				body, err = sjson.SetBytes(body, path, newText)
				if err != nil {
					continue
				}
				modified = true
			}
		}
	}

	if !modified {
		return body
	}
	return body
}

// StripTelemetryRequestHeaders removes telemetry-related headers from the request.
// 移除所有 Claude Code 客户端遥测相关请求头，包括设备指纹、会话标识、
// 客户端应用信息、语言偏好和浏览器安全头。
func StripTelemetryRequestHeaders(h http.Header) {
	// Collect keys to delete first (can't delete while iterating)
	var toDelete []string
	for k := range h {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-stainless-") ||
			lower == "user-agent" ||
			lower == "x-claude-code-session-id" ||
			lower == "x-client-request-id" ||
			lower == "x-claude-remote-container-id" ||
			lower == "x-claude-remote-session-id" ||
			lower == "x-client-app" ||
			lower == "x-app" ||
			lower == "x-anthropic-billing-header" ||
			lower == "anthropic-dangerous-direct-browser-access" ||
			lower == "anthropic-cc-fingerprint" ||
			lower == "accept-language" ||
			lower == "sec-fetch-mode" {
			toDelete = append(toDelete, k)
		}
	}
	for _, k := range toDelete {
		delete(h, k)
	}
}

// ApplyCanonicalHeaders sets minimal canonical header values for privacy mode.
// 设置最小化规范请求头值，移除所有用户/设备识别信息，
// 同时保持与 Anthropic API 的兼容性。
func ApplyCanonicalHeaders(h http.Header) {
	h.Set("User-Agent", "claude-cli/0.0.0 (privacy, cli)")
	h.Set("X-Stainless-OS", "unknown")
	h.Set("X-Stainless-Arch", "unknown")
	h.Set("X-Stainless-Runtime", "node")
	h.Set("X-Stainless-Runtime-Version", "0.0.0")
	h.Set("X-Stainless-Lang", "js")
	h.Set("X-Stainless-Package-Version", "0.0.0")
}
