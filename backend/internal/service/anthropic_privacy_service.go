package service

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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

// TelemetryPrivacyStats holds a snapshot of privacy action counters.
type TelemetryPrivacyStats struct {
	DroppedTelemetry int64 `json:"dropped_telemetry"`
	StrippedBody     int64 `json:"stripped_body"`
	StrippedHeaders  int64 `json:"stripped_headers"`
}

// GetTelemetryPrivacyStats returns a snapshot of the current privacy counters for a specific account.
func GetTelemetryPrivacyStats(accountID int64) TelemetryPrivacyStats {
	c := getAccountCounters(accountID)
	return TelemetryPrivacyStats{
		DroppedTelemetry: c.droppedTelemetry.Load(),
		StrippedBody:     c.strippedBody.Load(),
		StrippedHeaders:  c.strippedHeaders.Load(),
	}
}

// GetAllTelemetryPrivacyStats returns a snapshot of privacy counters for all accounts.
func GetAllTelemetryPrivacyStats() map[int64]TelemetryPrivacyStats {
	result := make(map[int64]TelemetryPrivacyStats)
	accountPrivacyStats.Range(func(key, value any) bool {
		id := key.(int64)
		c := value.(*accountPrivacyCounters)
		result[id] = TelemetryPrivacyStats{
			DroppedTelemetry: c.droppedTelemetry.Load(),
			StrippedBody:     c.strippedBody.Load(),
			StrippedHeaders:  c.strippedHeaders.Load(),
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

// ShouldDropTelemetryEndpoint returns true if the request should be silently dropped
// (return 200 without forwarding to upstream). Also returns the matched category.
func ShouldDropTelemetryEndpoint(path, host string) (drop bool, category string) {
	if strings.Contains(path, "/api/event_logging") {
		return true, "event_logging"
	}
	if strings.Contains(path, "/upstreamproxy") {
		return true, "upstream_proxy"
	}
	// Grove settings API — carries OAuth token and account metadata
	if strings.Contains(path, "/api/oauth/account") {
		return true, "grove_settings"
	}
	if strings.Contains(path, "/api/claude_code_grove") {
		return true, "grove_config"
	}
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

// RewriteTelemetryUserID rewrites the metadata.user_id field in the request body
// to use canonical privacy values.
func RewriteTelemetryUserID(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	userID := gjson.GetBytes(body, "metadata.user_id").String()
	if userID == "" {
		return body
	}

	_ = ParseMetadataUserID(userID) // verify format — overwrite regardless

	canonical := FormatMetadataUserID(
		domain.DefaultTelemetryPrivacyDeviceID,
		domain.DefaultTelemetryPrivacyAccountUUID,
		domain.DefaultTelemetryPrivacySessionID,
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
func ApplyCanonicalHeaders(h http.Header) {
	h.Set("User-Agent", "claude-cli/0.0.0 (privacy, cli)")
	h.Set("X-Stainless-OS", "unknown")
	h.Set("X-Stainless-Arch", "unknown")
	h.Set("X-Stainless-Runtime", "node")
	h.Set("X-Stainless-Runtime-Version", "0.0.0")
	h.Set("X-Stainless-Lang", "js")
	h.Set("X-Stainless-Package-Version", "0.0.0")
}