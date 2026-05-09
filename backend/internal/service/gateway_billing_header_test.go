package service

import (
	"fmt"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSyncBillingHeaderVersion(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		userAgent string
		wantSub   string // substring expected in result
		unchanged bool   // expect body to remain the same
	}{
		{
			name:      "replaces cc_version preserving message-derived suffix",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81.df2; cc_entrypoint=cli; cch=00000;"},{"type":"text","text":"You are Claude Code.","cache_control":{"type":"ephemeral"}}],"messages":[]}`,
			userAgent: "claude-cli/2.1.22 (external, cli)",
			wantSub:   "cc_version=2.1.22.df2",
		},
		{
			name:      "no billing header in system",
			body:      `{"system":[{"type":"text","text":"You are Claude Code."}],"messages":[]}`,
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
		{
			name:      "no system field",
			body:      `{"messages":[]}`,
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
		{
			name:      "user-agent without version",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent: "Mozilla/5.0",
			unchanged: true,
		},
		{
			name:      "empty user-agent",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent: "",
			unchanged: true,
		},
		{
			name:      "version already matches",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.22; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncBillingHeaderVersion([]byte(tt.body), tt.userAgent)
			if tt.unchanged {
				assert.Equal(t, tt.body, string(result), "body should remain unchanged")
			} else {
				assert.Contains(t, string(result), tt.wantSub)
				// Ensure old semver is gone
				assert.NotContains(t, string(result), "cc_version=2.1.81")
			}
		})
	}
}

func TestSignBillingHeaderCCH(t *testing.T) {
	t.Run("replaces placeholder with hash", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63.a43; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
		result := signBillingHeaderCCH(body)

		// Should not have the placeholder anymore
		assert.NotContains(t, string(result), "cch=00000")

		// Should have a 5 hex-char cch value
		billingText := gjson.GetBytes(result, "system.0.text").String()
		require.Contains(t, billingText, "cch=")
		assert.Regexp(t, `cch=[0-9a-f]{5};`, billingText)
	})

	t.Run("no placeholder - body unchanged", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=abcde;"}],"messages":[]}`)
		result := signBillingHeaderCCH(body)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("no billing header - body unchanged", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"You are Claude Code."}],"messages":[]}`)
		result := signBillingHeaderCCH(body)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("cch=00000 in user content is not touched", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":[{"type":"text","text":"keep literal cch=00000 in this message"}]}]}`)
		result := signBillingHeaderCCH(body)

		// Billing header should be signed
		billingText := gjson.GetBytes(result, "system.0.text").String()
		assert.NotContains(t, billingText, "cch=00000")

		// User message should keep its literal cch=00000
		userText := gjson.GetBytes(result, "messages.0.content.0.text").String()
		assert.Contains(t, userText, "cch=00000")
	})

	t.Run("signing is deterministic", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":"hi"}]}`)
		r1 := signBillingHeaderCCH(body)
		body2 := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":"hi"}]}`)
		r2 := signBillingHeaderCCH(body2)
		assert.Equal(t, string(r1), string(r2))
	})

	t.Run("matches reference algorithm", func(t *testing.T) {
		// Verify: signBillingHeaderCCH(body) produces cch = xxHash64(body_with_placeholder, seed) & 0xFFFFF
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63.a43; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
		expectedCCH := fmt.Sprintf("%05x", xxHash64Seeded(body, cchSeed)&0xFFFFF)

		result := signBillingHeaderCCH(body)
		billingText := gjson.GetBytes(result, "system.0.text").String()
		assert.Contains(t, billingText, "cch="+expectedCCH+";")
	})
}

func TestXXHash64Seeded(t *testing.T) {
	t.Run("matches cespare/xxhash for seed 0", func(t *testing.T) {
		inputs := []string{"", "a", "hello world", "The quick brown fox jumps over the lazy dog"}
		for _, s := range inputs {
			data := []byte(s)
			expected := xxhash.Sum64(data)
			got := xxHash64Seeded(data, 0)
			assert.Equal(t, expected, got, "mismatch for input %q", s)
		}
	})

	t.Run("large input matches cespare", func(t *testing.T) {
		data := make([]byte, 256)
		for i := range data {
			data[i] = byte(i)
		}
		expected := xxhash.Sum64(data)
		got := xxHash64Seeded(data, 0)
		assert.Equal(t, expected, got)
	})

	t.Run("deterministic with custom seed", func(t *testing.T) {
		data := []byte("hello world")
		h1 := xxHash64Seeded(data, cchSeed)
		h2 := xxHash64Seeded(data, cchSeed)
		assert.Equal(t, h1, h2)
	})

	t.Run("different seeds produce different results", func(t *testing.T) {
		data := []byte("test data for hashing")
		h1 := xxHash64Seeded(data, 0)
		h2 := xxHash64Seeded(data, cchSeed)
		assert.NotEqual(t, h1, h2)
	})
}

func TestForceBillingEntrypointForTelemetryPrivacy(t *testing.T) {
	enabledAccount := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"telemetry_privacy_enabled": true},
	}

	t.Run("启用遥测隐私时将 cc_entrypoint 改写为 cli", func(t *testing.T) {
		// 保护开启后 billing 头中的 sdk-cli entrypoint 应被收敛为 cli
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=sdk-cli; cch=00000;"}],"messages":[]}`)
		result := forceBillingEntrypointForTelemetryPrivacy(body, enabledAccount)
		billingText := gjson.GetBytes(result, "system.0.text").String()
		assert.Contains(t, billingText, "cc_entrypoint=cli")
		assert.NotContains(t, billingText, "cc_entrypoint=sdk-cli")
	})

	t.Run("已经是 cli 时幂等返回原 body", func(t *testing.T) {
		// 已收敛为 cli 的 billing 头不会被再次改写
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)
		result := forceBillingEntrypointForTelemetryPrivacy(body, enabledAccount)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("遥测隐私关闭时原样返回", func(t *testing.T) {
		// 即使 entrypoint 异常，关闭保护也不改写
		disabledAccount := &Account{
			ID:       1,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"telemetry_privacy_enabled": false},
		}
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=shell; cch=00000;"}],"messages":[]}`)
		result := forceBillingEntrypointForTelemetryPrivacy(body, disabledAccount)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("非 Anthropic OAuth 账号即使配置开关也不改写", func(t *testing.T) {
		// API Key 类型账号不在 IsTelemetryPrivacyEnabled 的作用范围内
		apiKeyAccount := &Account{
			ID:       1,
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"telemetry_privacy_enabled": true},
		}
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=sdk-cli; cch=00000;"}],"messages":[]}`)
		result := forceBillingEntrypointForTelemetryPrivacy(body, apiKeyAccount)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("仅作用于 system billing block，user 消息中的同名字面量不变", func(t *testing.T) {
		// cc_entrypoint= 出现在用户消息正文中的场景绝对不能被改写
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=sdk-cli; cch=00000;"}],"messages":[{"role":"user","content":[{"type":"text","text":"my shell script logs cc_entrypoint=sdk-cli in stderr"}]}]}`)
		result := forceBillingEntrypointForTelemetryPrivacy(body, enabledAccount)
		billingText := gjson.GetBytes(result, "system.0.text").String()
		assert.Contains(t, billingText, "cc_entrypoint=cli")
		userText := gjson.GetBytes(result, "messages.0.content.0.text").String()
		assert.Contains(t, userText, "cc_entrypoint=sdk-cli")
	})

	t.Run("nil 账号原样返回", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=sdk-cli; cch=00000;"}],"messages":[]}`)
		result := forceBillingEntrypointForTelemetryPrivacy(body, nil)
		assert.Equal(t, string(body), string(result))
	})
}

func TestStripBillingWorkloadForTelemetryPrivacy(t *testing.T) {
	enabledAccount := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"telemetry_privacy_enabled": true},
	}

	t.Run("启用遥测隐私时移除 cc_workload 段并保留前后字段", func(t *testing.T) {
		// cc_workload=cron; 段连同前导空白一并移除，cc_entrypoint 不受影响
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=cli; cc_workload=cron; cch=00000;"}],"messages":[]}`)
		result := stripBillingWorkloadForTelemetryPrivacy(body, enabledAccount)
		billingText := gjson.GetBytes(result, "system.0.text").String()
		assert.NotContains(t, billingText, "cc_workload")
		assert.Contains(t, billingText, "cc_entrypoint=cli")
		assert.Contains(t, billingText, "cc_version=2.1.92.a3f")
	})

	t.Run("无 cc_workload 时幂等返回原 body", func(t *testing.T) {
		// billing 头本就不带 workload，改写不会产生副作用
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)
		result := stripBillingWorkloadForTelemetryPrivacy(body, enabledAccount)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("遥测隐私关闭时即使存在 cc_workload 也不修改", func(t *testing.T) {
		disabledAccount := &Account{
			ID:       1,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"telemetry_privacy_enabled": false},
		}
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=cli; cc_workload=cron; cch=00000;"}],"messages":[]}`)
		result := stripBillingWorkloadForTelemetryPrivacy(body, disabledAccount)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("user 消息中包含 cc_workload= 字面量不会被剥离", func(t *testing.T) {
		// cc_workload= 出现在普通消息中时应原封不动
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=cli; cc_workload=cron; cch=00000;"}],"messages":[{"role":"user","content":[{"type":"text","text":"what is cc_workload=cron?"}]}]}`)
		result := stripBillingWorkloadForTelemetryPrivacy(body, enabledAccount)
		billingText := gjson.GetBytes(result, "system.0.text").String()
		assert.NotContains(t, billingText, "cc_workload")
		userText := gjson.GetBytes(result, "messages.0.content.0.text").String()
		assert.Contains(t, userText, "cc_workload=cron")
	})
}

func TestExtractBillingHeaderField(t *testing.T) {
	t.Run("读取 cc_entrypoint 与 cc_workload 字段值", func(t *testing.T) {
		// 正常从 billing header 块中提取各字段值
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=sdk-cli; cc_workload=cron; cch=00000;"}],"messages":[]}`)
		entrypoint := extractBillingHeaderField(body, "cc_entrypoint")
		assert.Equal(t, "sdk-cli", entrypoint)
		workload := extractBillingHeaderField(body, "cc_workload")
		assert.Equal(t, "cron", workload)
	})

	t.Run("缺失字段返回空串", func(t *testing.T) {
		// billing header 中没有对应 key 时返回空字符串
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92.a3f; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)
		workload := extractBillingHeaderField(body, "cc_workload")
		assert.Equal(t, "", workload)
	})

	t.Run("不在 billing header 块的同名字段不被读取", func(t *testing.T) {
		// user 消息中的 key=value 不会被误采集
		body := []byte(`{"system":[{"type":"text","text":"system prompt without billing header"}],"messages":[{"role":"user","content":[{"type":"text","text":"cc_entrypoint=malicious"}]}]}`)
		entrypoint := extractBillingHeaderField(body, "cc_entrypoint")
		assert.Equal(t, "", entrypoint)
	})
}
