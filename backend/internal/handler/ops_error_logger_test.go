package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func resetOpsErrorLoggerStateForTest(t *testing.T) {
	t.Helper()

	opsErrorLogMu.Lock()
	ch := opsErrorLogQueue
	opsErrorLogQueue = nil
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()

	if ch != nil {
		close(ch)
	}
	opsErrorLogWorkersWg.Wait()

	opsErrorLogOnce = sync.Once{}
	opsErrorLogStopOnce = sync.Once{}
	opsErrorLogWorkersWg = sync.WaitGroup{}
	opsErrorLogMu = sync.RWMutex{}
	opsErrorLogStopping = false

	opsErrorLogQueueLen.Store(0)
	opsErrorLogEnqueued.Store(0)
	opsErrorLogDropped.Store(0)
	opsErrorLogProcessed.Store(0)
	opsErrorLogSanitized.Store(0)
	opsErrorLogLastDropLogAt.Store(0)

	opsErrorLogShutdownCh = make(chan struct{})
	opsErrorLogShutdownOnce = sync.Once{}
	opsErrorLogDrained.Store(false)
}

func TestAttachOpsRequestBodyToEntry_SanitizeAndTrim(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	raw := []byte(`{"access_token":"secret-token","messages":[{"role":"user","content":"hello"}]}`)
	setOpsRequestContext(c, "claude-3", false, raw)

	entry := &service.OpsInsertErrorLogInput{}
	attachOpsRequestBodyToEntry(c, entry)

	require.NotNil(t, entry.RequestBodyBytes)
	require.Equal(t, len(raw), *entry.RequestBodyBytes)
	require.NotNil(t, entry.RequestBodyJSON)
	require.NotContains(t, *entry.RequestBodyJSON, "secret-token")
	require.Contains(t, *entry.RequestBodyJSON, "[REDACTED]")
	require.Equal(t, int64(1), OpsErrorLogSanitizedTotal())
}

func TestAttachOpsRequestBodyToEntry_InvalidJSONKeepsSize(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	raw := []byte("not-json")
	setOpsRequestContext(c, "claude-3", false, raw)

	entry := &service.OpsInsertErrorLogInput{}
	attachOpsRequestBodyToEntry(c, entry)

	require.Nil(t, entry.RequestBodyJSON)
	require.NotNil(t, entry.RequestBodyBytes)
	require.Equal(t, len(raw), *entry.RequestBodyBytes)
	require.False(t, entry.RequestBodyTruncated)
	require.Equal(t, int64(1), OpsErrorLogSanitizedTotal())
}

func TestApplyOpsTelemetryPrivacyErrorLogRedaction_ClearsModelAndBodyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	service.MarkOpsTelemetryPrivacySkipRequestBody(c)
	setOpsEndpointContext(c, "claude-3-7-sonnet-20250219", int16(2))

	// upstreamDetail 模拟上游原始响应体，包含模型名和遥测标识
	upstreamDetail := `{"model":"claude-3-7-sonnet-20250219","metadata":{"user_id":"raw-user"}}`
	// rawErrorBody 模拟 passthrough 场景下 ErrorBody 可能等同于上游原始响应体，
	// 包含 model key 和 metadata 子对象 —— 遥测脱敏应清除这些 key，保留 error 信息
	rawErrorBody := `{"model":"claude-3-7-sonnet-20250219","metadata":{"user_id":"raw-user","device_id":"dev-123"},"error":{"message":"No available accounts: no available accounts","type":"api_error"},"type":"error"}`
	entry := &service.OpsInsertErrorLogInput{
		Model:          "claude-3-7-sonnet-20250219",
		RequestedModel: "claude-3-7-sonnet-20250219",
		UpstreamModel:  getOpsUpstreamModelForLog(c),
		UserAgent:      "claude-cli/9.9.9 (external, cli)",
		ErrorMessage:   "No available accounts: no available accounts",
		ErrorBody:      rawErrorBody,
		UpstreamErrorMessage: func() *string {
			msg := "upstream model claude-3-7-sonnet-20250219 failed"
			return &msg
		}(),
		UpstreamErrorDetail: &upstreamDetail,
		RequestHeadersJSON:  &upstreamDetail,
		UpstreamErrors: []*service.OpsUpstreamErrorEvent{{
			UpstreamStatusCode:   http.StatusBadGateway,
			Kind:                 "request_error",
			Message:              "model claude-3-7-sonnet-20250219 failed",
			Detail:               upstreamDetail,
			UpstreamRequestBody:  upstreamDetail,
			UpstreamResponseBody: upstreamDetail,
		}},
	}

	applyOpsTelemetryPrivacyErrorLogRedaction(c, entry)

	// 模型名、User-Agent、请求头等身份相关字段必须清除
	require.Empty(t, entry.Model)
	require.Empty(t, entry.RequestedModel)
	require.Empty(t, entry.UpstreamModel)
	require.Empty(t, entry.UserAgent)
	require.Nil(t, entry.RequestHeadersJSON)
	// UpstreamErrorDetail 是上游响应体副本，在遥测隐私下必须清除
	require.Nil(t, entry.UpstreamErrorDetail)
	// ErrorMessage 是 JSON 解析后的错误消息，不含遥测标识，应保留
	require.Equal(t, "No available accounts: no available accounts", entry.ErrorMessage)
	// ErrorBody 经遥测专用脱敏后：model key 和 metadata 子对象已清除，error 信息保留
	require.NotEmpty(t, entry.ErrorBody)
	require.NotContains(t, entry.ErrorBody, `"model"`)
	require.NotContains(t, entry.ErrorBody, `"metadata"`)
	require.NotContains(t, entry.ErrorBody, "raw-user")
	require.NotContains(t, entry.ErrorBody, "dev-123")
	require.Contains(t, entry.ErrorBody, "api_error")
	require.Contains(t, entry.ErrorBody, "No available accounts")
	require.NotNil(t, entry.UpstreamErrorMessage)
	require.Equal(t, "upstream model claude-3-7-sonnet-20250219 failed", *entry.UpstreamErrorMessage)
	require.Len(t, entry.UpstreamErrors, 1)
	// 上游错误事件消息保留，Detail 清除（Detail 来自上游原始响应体）
	require.Equal(t, "model claude-3-7-sonnet-20250219 failed", entry.UpstreamErrors[0].Message)
	require.Empty(t, entry.UpstreamErrors[0].Detail)
	// 上游请求/响应正文必须清除，避免泄漏对话内容
	require.Empty(t, entry.UpstreamErrors[0].UpstreamRequestBody)
	require.Empty(t, entry.UpstreamErrors[0].UpstreamResponseBody)

	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	text := string(raw)
	// User-Agent 与上游原始响应体中的遥测标识必须在序列化后不可见
	require.NotContains(t, text, "claude-cli/9.9.9")
	require.NotContains(t, text, "raw-user")
	require.NotContains(t, text, "dev-123")
}

// TestApplyOpsTelemetryPrivacyErrorLogRedaction_NonJSONBodyClearsMessage 验证：
// ErrorBody 为非 JSON 时，parseOpsErrorResponse 会将原始响应体截断放入 ErrorMessage。
// 遥测隐私下 ErrorBody 被清空后，ErrorMessage 也必须同步清除，防止原始响应体通过 Message 落库。
func TestApplyOpsTelemetryPrivacyErrorLogRedaction_NonJSONBodyClearsMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	service.MarkOpsTelemetryPrivacySkipRequestBody(c)

	// 模拟 parseOpsErrorResponse 对非 JSON 响应的 fallback 行为：
	// 将原始 body 截断作为 ErrorMessage
	nonJSONBody := "<html>Internal Server Error: model claude-3-7-sonnet-20250219 for user raw-user</html>"
	entry := &service.OpsInsertErrorLogInput{
		Model:        "claude-3-7-sonnet-20250219",
		UserAgent:    "claude-cli/9.9.9 (external, cli)",
		ErrorMessage: nonJSONBody, // parseOpsErrorResponse 非 JSON fallback 的结果
		ErrorBody:    nonJSONBody,
	}

	applyOpsTelemetryPrivacyErrorLogRedaction(c, entry)

	// 非 JSON ErrorBody 被清空后，ErrorMessage 也必须同步清除
	require.Empty(t, entry.ErrorBody)
	require.Empty(t, entry.ErrorMessage)
	require.Empty(t, entry.Model)
	require.Empty(t, entry.UserAgent)
}

// TestApplyOpsTelemetryPrivacyErrorLogRedaction_JSONScalarBodyClearsMessage 验证：
// 响应体为合法 JSON scalar（如 `"raw string"`）时，parseOpsErrorResponse 会将其原文
// 放入 ErrorMessage，而 redactTelemetryPrivacyErrorBodyInPlace 将其视为非结构化响应清空，
// 从而触发 ErrorMessage 同步清除。
func TestApplyOpsTelemetryPrivacyErrorLogRedaction_JSONScalarBodyClearsMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	service.MarkOpsTelemetryPrivacySkipRequestBody(c)

	// JSON scalar body —— parseOpsErrorResponse 会 fallback 到原文
	jsonScalarBody := `"Internal error: model claude-3-7-sonnet for user raw-user"`
	entry := &service.OpsInsertErrorLogInput{
		Model:        "claude-3-7-sonnet-20250219",
		UserAgent:    "claude-cli/9.9.9 (external, cli)",
		ErrorMessage: jsonScalarBody,
		ErrorBody:    jsonScalarBody,
	}

	applyOpsTelemetryPrivacyErrorLogRedaction(c, entry)

	// JSON scalar 不是结构化响应，ErrorBody 和 ErrorMessage 必须同步清空
	require.Empty(t, entry.ErrorBody)
	require.Empty(t, entry.ErrorMessage)
	require.Empty(t, entry.Model)
	require.Empty(t, entry.UserAgent)
}

func TestRedactTelemetryPrivacyErrorBodyInPlace_EmptyReturnsFalse(t *testing.T) {
	entry := &service.OpsInsertErrorLogInput{ErrorBody: ""}
	require.False(t, redactTelemetryPrivacyErrorBodyInPlace(entry))
	require.Empty(t, entry.ErrorBody)

	nilEntry := (*service.OpsInsertErrorLogInput)(nil)
	require.False(t, redactTelemetryPrivacyErrorBodyInPlace(nilEntry))
}

func TestRedactTelemetryPrivacyErrorBodyInPlace_PreservesErrorStructure(t *testing.T) {
	entry := &service.OpsInsertErrorLogInput{
		ErrorBody: `{"code":"INSUFFICIENT_BALANCE","message":"Insufficient balance","type":"error"}`,
	}
	require.True(t, redactTelemetryPrivacyErrorBodyInPlace(entry))
	require.NotEmpty(t, entry.ErrorBody)
	require.Contains(t, entry.ErrorBody, "INSUFFICIENT_BALANCE")
	require.Contains(t, entry.ErrorBody, "Insufficient balance")
	require.NotContains(t, entry.ErrorBody, `"model"`)
}

func TestRedactTelemetryPrivacyErrorBodyInPlace_CaseInsensitiveKeys(t *testing.T) {
	entry := &service.OpsInsertErrorLogInput{
		ErrorBody: `{"Model":"claude-3-7-sonnet","Metadata":{"user_id":"u1"},"error":{"type":"api_error","message":"fail"}}`,
	}
	require.True(t, redactTelemetryPrivacyErrorBodyInPlace(entry))
	require.NotEmpty(t, entry.ErrorBody)
	require.NotContains(t, entry.ErrorBody, `"Model"`)
	require.NotContains(t, entry.ErrorBody, `"Metadata"`)
	require.NotContains(t, entry.ErrorBody, "claude-3-7-sonnet")
	require.NotContains(t, entry.ErrorBody, "u1")
	require.Contains(t, entry.ErrorBody, "api_error")
}

func TestMarkOpsTelemetryPrivacyForAccount_ClearsExistingRequestContext(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("anthropic-beta", "claude-code-20250219")

	raw := []byte(`{"model":"claude-3-7-sonnet-20250219","metadata":{"user_id":"raw-user"},"messages":[{"role":"user","content":"hello"}]}`)
	setOpsRequestContext(c, "claude-3-7-sonnet-20250219", true, raw)
	setOpsEndpointContext(c, "claude-3-7-sonnet-20250219", int16(2))

	account := &service.Account{
		ID:       4,
		Platform: service.PlatformAnthropic,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			"telemetry_privacy_enabled": true,
		},
	}
	markOpsTelemetryPrivacyForAccount(c, account)

	require.True(t, service.ShouldSkipOpsRequestBodyForTelemetryPrivacy(c))
	require.Equal(t, "", c.Request.Context().Value(ctxkey.Model))
	require.Equal(t, "", getOpsUpstreamModelForLog(c))

	model, ok := c.Get(opsModelKey)
	require.True(t, ok)
	require.Equal(t, "", model)
	storedBody, ok := c.Get(opsRequestBodyKey)
	require.True(t, ok)
	require.Empty(t, storedBody)

	entry := &service.OpsInsertErrorLogInput{
		Model:          "claude-3-7-sonnet-20250219",
		RequestedModel: "claude-3-7-sonnet-20250219",
		UpstreamModel:  "claude-3-7-sonnet-20250219",
		UserAgent:      "claude-cli/9.9.9 (external, cli)",
		ErrorMessage:   "model claude-3-7-sonnet-20250219 failed",
	}
	entry.RequestHeadersJSON = extractOpsRetryRequestHeaders(c)
	attachOpsRequestBodyToEntry(c, entry)
	applyOpsTelemetryPrivacyErrorLogRedaction(c, entry)

	require.Nil(t, entry.RequestHeadersJSON)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)
	require.Empty(t, entry.Model)
	require.Empty(t, entry.RequestedModel)
	require.Empty(t, entry.UpstreamModel)
	require.Empty(t, entry.UserAgent)
	// 错误消息必须保留，确保管理员可定位问题根因
	require.Equal(t, "model claude-3-7-sonnet-20250219 failed", entry.ErrorMessage)
	require.Equal(t, int64(0), OpsErrorLogSanitizedTotal())
}

func TestMarkOpsTelemetryPrivacyForAccount_IgnoresNonPrivacyAccounts(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	raw := []byte(`{"model":"claude-3-7-sonnet-20250219"}`)
	setOpsRequestContext(c, "claude-3-7-sonnet-20250219", false, raw)

	account := &service.Account{
		ID:       5,
		Platform: service.PlatformAnthropic,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			"telemetry_privacy_enabled": false,
		},
	}
	markOpsTelemetryPrivacyForAccount(c, account)

	require.False(t, service.ShouldSkipOpsRequestBodyForTelemetryPrivacy(c))
	require.Equal(t, "claude-3-7-sonnet-20250219", c.Request.Context().Value(ctxkey.Model))

	entry := &service.OpsInsertErrorLogInput{}
	attachOpsRequestBodyToEntry(c, entry)

	require.NotNil(t, entry.RequestBodyJSON)
	require.NotNil(t, entry.RequestBodyBytes)
	require.Equal(t, int64(1), OpsErrorLogSanitizedTotal())
}

func TestSetOpsRequestContext_SkipTelemetryPrivacyDoesNotStoreLaterBody(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	service.MarkOpsTelemetryPrivacySkipRequestBody(c)

	setOpsRequestContext(c, "claude-3-7-sonnet-20250219", true, []byte(`{"model":"claude-3-7-sonnet-20250219"}`))
	setOpsEndpointContext(c, "claude-3-7-sonnet-20250219", int16(2))

	model, ok := c.Get(opsModelKey)
	require.True(t, ok)
	require.Equal(t, "", model)
	_, ok = c.Get(opsRequestBodyKey)
	require.False(t, ok)
	require.Equal(t, "", getOpsUpstreamModelForLog(c))
	require.Nil(t, extractOpsRetryRequestHeaders(c))

	entry := &service.OpsInsertErrorLogInput{}
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)
	require.Equal(t, int64(0), OpsErrorLogSanitizedTotal())
}

func TestEnqueueOpsErrorLog_QueueFullDrop(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	// 禁止 enqueueOpsErrorLog 触发 workers，使用测试队列验证满队列降级。
	opsErrorLogOnce.Do(func() {})

	opsErrorLogMu.Lock()
	opsErrorLogQueue = make(chan opsErrorLogJob, 1)
	opsErrorLogMu.Unlock()

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"}

	enqueueOpsErrorLog(ops, entry)
	enqueueOpsErrorLog(ops, entry)

	require.Equal(t, int64(1), OpsErrorLogEnqueuedTotal())
	require.Equal(t, int64(1), OpsErrorLogDroppedTotal())
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
}

func TestAttachOpsRequestBodyToEntry_EarlyReturnBranches(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	entry := &service.OpsInsertErrorLogInput{}
	attachOpsRequestBodyToEntry(nil, entry)
	attachOpsRequestBodyToEntry(&gin.Context{}, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 无请求体 key
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)
	require.False(t, entry.RequestBodyTruncated)

	// 错误类型
	c.Set(opsRequestBodyKey, "not-bytes")
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)

	// 空 bytes
	c.Set(opsRequestBodyKey, []byte{})
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)

	require.Equal(t, int64(0), OpsErrorLogSanitizedTotal())
}

func TestEnqueueOpsErrorLog_EarlyReturnBranches(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"}

	// nil 入参分支
	enqueueOpsErrorLog(nil, entry)
	enqueueOpsErrorLog(ops, nil)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// shutdown 分支
	close(opsErrorLogShutdownCh)
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// stopping 分支
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogMu.Lock()
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// queue nil 分支（防止启动 worker 干扰）
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogOnce.Do(func() {})
	opsErrorLogMu.Lock()
	opsErrorLogQueue = nil
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())
}

func TestOpsCaptureWriterPool_ResetOnRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	writer := acquireOpsCaptureWriter(c.Writer)
	require.NotNil(t, writer)
	_, err := writer.buf.WriteString("temp-error-body")
	require.NoError(t, err)

	releaseOpsCaptureWriter(writer)

	reused := acquireOpsCaptureWriter(c.Writer)
	defer releaseOpsCaptureWriter(reused)

	require.Zero(t, reused.buf.Len(), "writer should be reset before reuse")
}

func TestOpsErrorLoggerMiddleware_DoesNotBreakOuterMiddlewares(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware2.Recovery())
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	r.GET("/v1/messages", OpsErrorLoggerMiddleware(nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)

	require.NotPanics(t, func() {
		r.ServeHTTP(rec, req)
	})
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestIsKnownOpsErrorType(t *testing.T) {
	known := []string{
		"invalid_request_error",
		"authentication_error",
		"rate_limit_error",
		"billing_error",
		"subscription_error",
		"upstream_error",
		"overloaded_error",
		"api_error",
		"not_found_error",
		"forbidden_error",
	}
	for _, k := range known {
		require.True(t, isKnownOpsErrorType(k), "expected known: %s", k)
	}

	unknown := []string{"<nil>", "null", "", "random_error", "some_new_type", "<nil>>"}
	for _, u := range unknown {
		require.False(t, isKnownOpsErrorType(u), "expected unknown: %q", u)
	}
}

func TestNormalizeOpsErrorType(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		code    string
		want    string
	}{
		// Known types pass through.
		{"known invalid_request_error", "invalid_request_error", "", "invalid_request_error"},
		{"known rate_limit_error", "rate_limit_error", "", "rate_limit_error"},
		{"known upstream_error", "upstream_error", "", "upstream_error"},

		// Unknown/garbage types are rejected and fall through to code-based or default.
		{"nil literal from upstream", "<nil>", "", "api_error"},
		{"null string", "null", "", "api_error"},
		{"random string", "something_weird", "", "api_error"},

		// Unknown type but known code still maps correctly.
		{"nil with INSUFFICIENT_BALANCE code", "<nil>", "INSUFFICIENT_BALANCE", "billing_error"},
		{"nil with USAGE_LIMIT_EXCEEDED code", "<nil>", "USAGE_LIMIT_EXCEEDED", "subscription_error"},

		// Empty type falls through to code-based mapping.
		{"empty type with balance code", "", "INSUFFICIENT_BALANCE", "billing_error"},
		{"empty type with subscription code", "", "SUBSCRIPTION_NOT_FOUND", "subscription_error"},
		{"empty type no code", "", "", "api_error"},

		// Known type overrides conflicting code-based mapping.
		{"known type overrides conflicting code", "rate_limit_error", "INSUFFICIENT_BALANCE", "rate_limit_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeOpsErrorType(tt.errType, tt.code)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSetOpsEndpointContext_SetsContextKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	setOpsEndpointContext(c, "claude-3-5-sonnet-20241022", int16(2)) // stream

	v, ok := c.Get(opsUpstreamModelKey)
	require.True(t, ok)
	vStr, ok := v.(string)
	require.True(t, ok)
	require.Equal(t, "claude-3-5-sonnet-20241022", vStr)

	rt, ok := c.Get(opsRequestTypeKey)
	require.True(t, ok)
	rtVal, ok := rt.(int16)
	require.True(t, ok)
	require.Equal(t, int16(2), rtVal)
}

func TestSetOpsEndpointContext_EmptyModelNotStored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	setOpsEndpointContext(c, "", int16(1))

	_, ok := c.Get(opsUpstreamModelKey)
	require.False(t, ok, "empty upstream model should not be stored")

	rt, ok := c.Get(opsRequestTypeKey)
	require.True(t, ok)
	rtVal, ok := rt.(int16)
	require.True(t, ok)
	require.Equal(t, int16(1), rtVal)
}

func TestSetOpsEndpointContext_NilContext(t *testing.T) {
	require.NotPanics(t, func() {
		setOpsEndpointContext(nil, "model", int16(1))
	})
}
