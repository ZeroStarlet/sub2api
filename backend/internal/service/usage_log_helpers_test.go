package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRedactUsageLogIdentityForTelemetryPrivacy 验证 usage_logs 写入路径在不同账号
// 配置下对 user_agent 与 ip_address 的脱敏行为，覆盖正向、关闭、非目标平台与异常入参。
//
// 这个 helper 是 buildRecordUsageLog 链路上唯一保护本地数据库不再持久化客户端可识别字段
// 的关卡，因此每个分支必须有显式断言；任何分支静默放行都会导致 usage_logs 表出现客户端
// 真实 User-Agent 或 IP，破坏遥测隐私功能的承诺。
func TestRedactUsageLogIdentityForTelemetryPrivacy(t *testing.T) {
	const sampleUserAgent = "claude-cli/2.1.92 (external, cli)"
	const sampleIPAddress = "203.0.113.7"

	t.Run("启用遥测隐私的 Anthropic OAuth 账号会清空 UA 与 IP", func(t *testing.T) {
		account := &Account{
			ID:       1001,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"telemetry_privacy_enabled": true,
			},
		}

		ua, ip := redactUsageLogIdentityForTelemetryPrivacy(account, sampleUserAgent, sampleIPAddress)

		require.Equal(t, "", ua, "启用遥测隐私后 usage_logs.user_agent 必须落库为空，避免客户端指纹持久化")
		require.Equal(t, "", ip, "启用遥测隐私后 usage_logs.ip_address 必须落库为空，避免客户端 IP 持久化")
	})

	t.Run("启用遥测隐私的 Anthropic Setup Token 账号会清空 UA 与 IP", func(t *testing.T) {
		account := &Account{
			ID:       1002,
			Platform: PlatformAnthropic,
			Type:     AccountTypeSetupToken,
			Extra: map[string]any{
				"telemetry_privacy_enabled": true,
			},
		}

		ua, ip := redactUsageLogIdentityForTelemetryPrivacy(account, sampleUserAgent, sampleIPAddress)

		require.Equal(t, "", ua, "Setup Token 与 OAuth 类型在遥测隐私维度应等价，必须清空 UA")
		require.Equal(t, "", ip, "Setup Token 与 OAuth 类型在遥测隐私维度应等价，必须清空 IP")
	})

	t.Run("未启用遥测隐私的账号保持原值", func(t *testing.T) {
		account := &Account{
			ID:       1003,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		}

		ua, ip := redactUsageLogIdentityForTelemetryPrivacy(account, sampleUserAgent, sampleIPAddress)

		require.Equal(t, sampleUserAgent, ua, "未启用遥测隐私时必须原样落库，否则历史 dashboard 与排障链路会被破坏")
		require.Equal(t, sampleIPAddress, ip, "未启用遥测隐私时 IP 必须原样落库，保持运维可见性")
	})

	t.Run("nil 账号视为未启用并原样返回", func(t *testing.T) {
		ua, ip := redactUsageLogIdentityForTelemetryPrivacy(nil, sampleUserAgent, sampleIPAddress)

		require.Equal(t, sampleUserAgent, ua, "nil 账号属于异常路径但不应吞掉 UA，由 buildRecordUsageLog 上游报错")
		require.Equal(t, sampleIPAddress, ip, "nil 账号属于异常路径但不应吞掉 IP，避免与真正的脱敏分支混淆")
	})

	t.Run("非 Anthropic 账号即使配置了开关也不会被脱敏", func(t *testing.T) {
		account := &Account{
			ID:       1004,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"telemetry_privacy_enabled": true,
			},
		}

		ua, ip := redactUsageLogIdentityForTelemetryPrivacy(account, sampleUserAgent, sampleIPAddress)

		require.Equal(t, sampleUserAgent, ua, "OpenAI 等非 Anthropic 账号不在该功能作用域，必须保持原值")
		require.Equal(t, sampleIPAddress, ip, "非 Anthropic 账号不应被遥测隐私功能影响，避免误改非目标链路")
	})

	t.Run("Anthropic API Key 账号即使配置了开关也不会被脱敏", func(t *testing.T) {
		account := &Account{
			ID:       1005,
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"telemetry_privacy_enabled": true,
			},
		}

		ua, ip := redactUsageLogIdentityForTelemetryPrivacy(account, sampleUserAgent, sampleIPAddress)

		require.Equal(t, sampleUserAgent, ua, "API Key 类型不依赖客户端遥测身份，遥测隐私功能不针对该路径生效")
		require.Equal(t, sampleIPAddress, ip, "API Key 路径与 OAuth 风险面不同，必须按设计保持原值")
	})

	t.Run("启用遥测隐私时空入参依然返回空串保持幂等", func(t *testing.T) {
		account := &Account{
			ID:       1006,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"telemetry_privacy_enabled": true,
			},
		}

		ua, ip := redactUsageLogIdentityForTelemetryPrivacy(account, "", "")

		require.Equal(t, "", ua, "对已脱敏空串再次调用应是幂等的")
		require.Equal(t, "", ip, "对已脱敏空串再次调用应是幂等的")
	})
}

// TestBuildRecordUsageLog_TelemetryPrivacyAppliedAtCallSite 锁定 buildRecordUsageLog 调用点行为，
// 防止未来重构时 helper 调用被静默移除导致 usage_logs 重新落库客户端 UA / IP。
//
// 设计取舍：
//   - 该测试不依赖 GatewayService 的任何注入字段（cfg、cache、billingService 均不参与
//     buildRecordUsageLog 内部组装），因此使用零值 GatewayService 即可，无需引入
//     mock 体系，也避免与其它子系统的初始化耦合
//   - 用 telemetryPrivacyAnthropicAccount() 共享 fixture 保持遥测隐私帐号定义单点维护
//   - 提供"启用脱敏"和"未启用脱敏（关闭开关）"两条对照路径，覆盖回退能力的核心断言：
//     关闭后下一次写入立即恢复原值，不依赖 schema 改动或缓存清理
func TestBuildRecordUsageLog_TelemetryPrivacyAppliedAtCallSite(t *testing.T) {
	const sampleUserAgent = "claude-cli/2.1.92 (external, cli)"
	const sampleIPAddress = "203.0.113.7"

	// buildRecordUsageLog 内部仅依赖入参，不访问 receiver 字段，所以零值 GatewayService 即可。
	svc := &GatewayService{}
	ctx := context.Background()

	apiKey := &APIKey{ID: 901}
	user := &User{ID: 17}
	result := &ForwardResult{
		RequestID: "req-telemetry-privacy-call-site",
		Model:     "claude-3-7-sonnet-20250219",
		Duration:  time.Second,
	}

	t.Run("启用遥测隐私的 Anthropic OAuth 账号导致 UsageLog 落 nil", func(t *testing.T) {
		account := telemetryPrivacyAnthropicAccount()
		input := &recordUsageCoreInput{
			Result:    result,
			APIKey:    apiKey,
			User:      user,
			Account:   account,
			UserAgent: sampleUserAgent,
			IPAddress: sampleIPAddress,
		}

		usageLog := svc.buildRecordUsageLog(
			ctx,
			input,
			result,
			apiKey,
			user,
			account,
			nil, // 无订阅，等价于普通 token 计费路径
			result.Model,
			1.0,
			1.0,
			1.0,
			0,
			false,
			nil, // 无成本对象，buildRecordUsageLog 内部走默认零值组装
			&recordUsageOpts{},
		)

		require.NotNil(t, usageLog, "buildRecordUsageLog 必须返回非 nil 的 UsageLog 用于落库")
		require.Nil(t, usageLog.UserAgent, "启用遥测隐私后 UsageLog.UserAgent 必须为 nil 以避免 user_agent 列写入客户端指纹")
		require.Nil(t, usageLog.IPAddress, "启用遥测隐私后 UsageLog.IPAddress 必须为 nil 以避免 ip_address 列写入客户端 IP")
	})

	t.Run("关闭遥测隐私的 Anthropic OAuth 账号下次落库立即恢复原值", func(t *testing.T) {
		account := telemetryPrivacyAnthropicAccount()
		// 模拟运维侧关闭开关：可回退能力核心断言
		account.Extra["telemetry_privacy_enabled"] = false

		input := &recordUsageCoreInput{
			Result:    result,
			APIKey:    apiKey,
			User:      user,
			Account:   account,
			UserAgent: sampleUserAgent,
			IPAddress: sampleIPAddress,
		}

		usageLog := svc.buildRecordUsageLog(
			ctx,
			input,
			result,
			apiKey,
			user,
			account,
			nil,
			result.Model,
			1.0,
			1.0,
			1.0,
			0,
			false,
			nil,
			&recordUsageOpts{},
		)

		require.NotNil(t, usageLog)
		require.NotNil(t, usageLog.UserAgent, "关闭遥测隐私后必须立即恢复原值落库，否则违反可回退要求")
		require.NotNil(t, usageLog.IPAddress, "关闭遥测隐私后 IP 必须立即恢复落库，保持运维可见性")
		require.Equal(t, sampleUserAgent, *usageLog.UserAgent, "关闭后 UA 必须与入参完全一致")
		require.Equal(t, sampleIPAddress, *usageLog.IPAddress, "关闭后 IP 必须与入参完全一致")
	})
}
