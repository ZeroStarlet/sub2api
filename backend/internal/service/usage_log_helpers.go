package service

import "strings"

func optionalTrimmedStringPtr(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// redactUsageLogIdentityForTelemetryPrivacy 在写入 usage_logs 之前，根据账号级遥测隐私开关
// 决定是否清空客户端可识别字段。该函数只针对 Anthropic OAuth/SetupToken 且显式启用遥测隐私
// 保护的账号生效；其它账号（API Key、Bedrock、Vertex Service Account 以及非 Anthropic 平台）
// 会原样返回输入值，避免误改非目标链路。
//
// 参数说明：
//   - account：本次请求最终选中的上游账号；nil 视为未启用，原样返回入参
//   - userAgent：handler 层从客户端原始 User-Agent 头复制得到的字符串
//   - ipAddress：handler 层从可信 X-Forwarded-For/RemoteAddr 解析出的客户端 IP
//
// 返回值：
//   - 第一个返回值是脱敏后的 User-Agent（启用遥测隐私时为空字符串，调用方可透传给
//     optionalTrimmedStringPtr 写成数据库 NULL）
//   - 第二个返回值是脱敏后的 IP，与 User-Agent 同步处理
//
// 边界条件：
//   - 账号未启用遥测隐私、不是 Anthropic OAuth/SetupToken 时，直接返回原值（包括空串）
//   - 多次调用幂等：再次传入已脱敏的空串，输出仍是空串
//   - 不会修改 account 或其它共享对象，可安全在并发请求中调用
//
// 调用面与覆盖范围：
//   - 当前唯一调用点是 GatewayService.recordUsageCore → buildRecordUsageLog 共享写入路径
//     （gateway_service.go），Anthropic 标准入口、Gemini、Antigravity 都经过此路径
//   - openai_gateway_service.go 维护独立的 UsageLog 构造，但只调度 PlatformOpenAI 账号，
//     由于 IsTelemetryPrivacyEnabled() 内部要求 IsAnthropicOAuthOrSetupToken()，
//     即使误传入也会自然 NOOP；该旁路不在本函数承诺的覆盖面之内
//   - usage_service.go 直接落 UsageLog 的路径不写 UserAgent/IPAddress，无暴露面
//
// 设计取舍：
//   - 选择在 service 层一处统一拦截 Anthropic 共享落库路径，避免每个 handler 重复加判断
//   - 不改变 usage_logs 的列结构，遥测隐私关闭时与历史行为完全一致，便于无缝回退
//   - 与上游请求转发链路解耦：上游请求中 User-Agent 由 buildUpstreamRequest 单独替换，
//     此处只负责本地存储侧脱敏，二者各司其职以避免互相影响
func redactUsageLogIdentityForTelemetryPrivacy(account *Account, userAgent, ipAddress string) (string, string) {
	if account == nil || !account.IsTelemetryPrivacyEnabled() {
		return userAgent, ipAddress
	}
	return "", ""
}

// optionalNonEqualStringPtr returns a pointer to value if it is non-empty and
// differs from compare; otherwise nil. Used to store upstream_model only when
// it differs from the requested model.
func optionalNonEqualStringPtr(value, compare string) *string {
	if value == "" || value == compare {
		return nil
	}
	return &value
}

func forwardResultBillingModel(requestedModel, upstreamModel string) string {
	if trimmed := strings.TrimSpace(requestedModel); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(upstreamModel)
}

func optionalInt64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
