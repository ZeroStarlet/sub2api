package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type telemetryPrivacyIdentityCacheStub struct {
	fingerprint     *Fingerprint
	maskedSessionID string
}

func (s *telemetryPrivacyIdentityCacheStub) GetFingerprint(_ context.Context, _ int64) (*Fingerprint, error) {
	return s.fingerprint, nil
}

func (s *telemetryPrivacyIdentityCacheStub) SetFingerprint(_ context.Context, _ int64, fp *Fingerprint) error {
	s.fingerprint = fp
	return nil
}

func (s *telemetryPrivacyIdentityCacheStub) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return s.maskedSessionID, nil
}

func (s *telemetryPrivacyIdentityCacheStub) SetMaskedSessionID(_ context.Context, _ int64, sessionID string) error {
	s.maskedSessionID = sessionID
	return nil
}

type telemetryPrivacyFailingIdentityCacheStub struct{}

func (s *telemetryPrivacyFailingIdentityCacheStub) GetFingerprint(_ context.Context, _ int64) (*Fingerprint, error) {
	return nil, errors.New("指纹缓存不可用")
}

func (s *telemetryPrivacyFailingIdentityCacheStub) SetFingerprint(_ context.Context, _ int64, _ *Fingerprint) error {
	return errors.New("指纹缓存不可写")
}

func (s *telemetryPrivacyFailingIdentityCacheStub) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return "", nil
}

func (s *telemetryPrivacyFailingIdentityCacheStub) SetMaskedSessionID(_ context.Context, _ int64, _ string) error {
	return nil
}

type telemetryPrivacyCounterRepoStub struct {
	AccountRepository
	counts map[int64]int64
}

func (s *telemetryPrivacyCounterRepoStub) IncrementExtraCounter(_ context.Context, id int64, key string, delta int64) error {
	if s.counts == nil {
		s.counts = make(map[int64]int64)
	}
	if key != AccountExtraTelemetryPrivacyProtectedCount {
		return nil
	}
	s.counts[id] += delta
	return nil
}

type telemetryPrivacyLogSinkStub struct {
	events []*logger.LogEvent
}

func (s *telemetryPrivacyLogSinkStub) WriteLogEvent(event *logger.LogEvent) {
	s.events = append(s.events, event)
}

func TestAccount_IsTelemetryPrivacyEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "anthropic oauth enabled",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"telemetry_privacy_enabled": true,
				},
			},
			want: true,
		},
		{
			name: "anthropic setup token enabled",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeSetupToken,
				Extra: map[string]any{
					"telemetry_privacy_enabled": true,
				},
			},
			want: true,
		},
		{
			name: "api key ignored",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"telemetry_privacy_enabled": true,
				},
			},
			want: false,
		},
		{
			name: "other platform ignored",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"telemetry_privacy_enabled": true,
				},
			},
			want: false,
		},
		{
			name: "invalid value disabled",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"telemetry_privacy_enabled": "true",
				},
			},
			want: false,
		},
		{
			name:    "nil account disabled",
			account: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsTelemetryPrivacyEnabled())
		})
	}
}

func TestSanitizeAnthropicTelemetryPrivacyBody_UsesSingleAccountSession(t *testing.T) {
	account := &Account{
		ID:       42,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"telemetry_privacy_enabled": true,
		},
	}
	fp := &Fingerprint{
		ClientID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		UserAgent: "claude-cli/2.1.92 (external, cli)",
	}

	bodyA := telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000")
	bodyB := telemetryPrivacyRequestBody("2222222222222222222222222222222222222222222222222222222222222222", "550e8400-e29b-41d4-a716-446655440000", "223e4567-e89b-12d3-a456-426614174000")

	resultA, parsedA, protectedA := sanitizeAnthropicTelemetryPrivacyBody(bodyA, account)
	resultB, parsedB, protectedB := sanitizeAnthropicTelemetryPrivacyBody(bodyB, account)

	require.NotEqual(t, string(bodyA), string(resultA))
	require.NotEqual(t, string(bodyB), string(resultB))
	require.True(t, protectedA)
	require.True(t, protectedB)
	require.NotNil(t, parsedA)
	require.NotNil(t, parsedB)
	require.Equal(t, anthropicTelemetryPrivacyDeviceID(account), parsedA.DeviceID)
	require.Equal(t, anthropicTelemetryPrivacyDeviceID(account), parsedB.DeviceID)
	require.NotEqual(t, fp.ClientID, parsedA.DeviceID)
	require.Empty(t, parsedA.AccountUUID)
	require.Empty(t, parsedB.AccountUUID)
	require.Equal(t, anthropicTelemetryPrivacySessionID(account), parsedA.SessionID)
	require.Equal(t, parsedA.SessionID, parsedB.SessionID)
	require.NotContains(t, string(resultA), "550e8400-e29b-41d4-a716-446655440000")
	require.NotContains(t, string(resultA), "123e4567-e89b-12d3-a456-426614174000")
}

func TestAccount_GetTelemetryPrivacyProtectedCount(t *testing.T) {
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			AccountExtraTelemetryPrivacyProtectedCount: "12",
		},
	}
	require.Equal(t, int64(12), account.GetTelemetryPrivacyProtectedCount())

	account.Extra[AccountExtraTelemetryPrivacyProtectedCount] = -1
	require.Zero(t, account.GetTelemetryPrivacyProtectedCount())

	account.Platform = PlatformOpenAI
	account.Extra[AccountExtraTelemetryPrivacyProtectedCount] = 12
	require.Zero(t, account.GetTelemetryPrivacyProtectedCount())
}

func TestDeferredService_TelemetryPrivacyProtectionCountsAccumulate(t *testing.T) {
	repo := &telemetryPrivacyCounterRepoStub{}
	svc := NewDeferredService(repo, nil, time.Second)

	svc.ScheduleTelemetryPrivacyProtection(42)
	svc.ScheduleTelemetryPrivacyProtection(42)
	svc.ScheduleTelemetryPrivacyProtection(99)
	svc.flushTelemetryPrivacyProtectionCounts()

	require.Equal(t, int64(2), repo.counts[42])
	require.Equal(t, int64(1), repo.counts[99])
}

func TestGatewayService_BuildUpstreamRequest_AppliesTelemetryPrivacy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := telemetryPrivacyAnthropicAccount()
	fp := telemetryPrivacyFingerprint()
	cache := &telemetryPrivacyIdentityCacheStub{fingerprint: fp}
	svc := &GatewayService{identityService: NewIdentityService(cache)}
	c := telemetryPrivacyGinContext()

	req, err := svc.buildUpstreamRequest(
		context.Background(),
		c,
		account,
		telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
		false,
	)
	require.NoError(t, err)

	body := telemetryPrivacyReadRequestBody(t, req)
	parsed := ParseMetadataUserID(gjson.GetBytes(body, "metadata.user_id").String())
	require.NotNil(t, parsed)
	require.Equal(t, anthropicTelemetryPrivacyDeviceID(account), parsed.DeviceID)
	require.Empty(t, parsed.AccountUUID)
	require.Equal(t, anthropicTelemetryPrivacySessionID(account), parsed.SessionID)
	require.Equal(t, parsed.SessionID, getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"))
	require.NotEqual(t, "client-request-id-real", getHeaderRaw(req.Header, "x-client-request-id"))
	require.NotEmpty(t, getHeaderRaw(req.Header, "x-client-request-id"))
	require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, claude.DefaultHeaders["X-Stainless-OS"], getHeaderRaw(req.Header, "X-Stainless-OS"))
	require.Equal(t, "Bearer oauth-token", getHeaderRaw(req.Header, "authorization"))
	require.NotContains(t, string(body), "550e8400-e29b-41d4-a716-446655440000")
	require.NotContains(t, string(body), "123e4567-e89b-12d3-a456-426614174000")
}

func TestGatewayService_BuildUpstreamRequest_TelemetryPrivacyUsesStableDeviceWhenFingerprintCacheFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := telemetryPrivacyAnthropicAccount()
	svc := &GatewayService{identityService: NewIdentityService(&telemetryPrivacyFailingIdentityCacheStub{})}

	reqA, err := svc.buildUpstreamRequest(
		context.Background(),
		telemetryPrivacyGinContext(),
		account,
		telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
		false,
	)
	require.NoError(t, err)
	reqB, err := svc.buildUpstreamRequest(
		context.Background(),
		telemetryPrivacyGinContext(),
		account,
		telemetryPrivacyRequestBody("2222222222222222222222222222222222222222222222222222222222222222", "550e8400-e29b-41d4-a716-446655440000", "223e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
		false,
	)
	require.NoError(t, err)

	parsedA := ParseMetadataUserID(gjson.GetBytes(telemetryPrivacyReadRequestBody(t, reqA), "metadata.user_id").String())
	parsedB := ParseMetadataUserID(gjson.GetBytes(telemetryPrivacyReadRequestBody(t, reqB), "metadata.user_id").String())
	require.NotNil(t, parsedA)
	require.NotNil(t, parsedB)
	require.Equal(t, anthropicTelemetryPrivacyDeviceID(account), parsedA.DeviceID)
	require.Equal(t, parsedA.DeviceID, parsedB.DeviceID)
	require.Equal(t, parsedA.SessionID, parsedB.SessionID)
	require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(reqA.Header, "User-Agent"))
	require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(reqB.Header, "User-Agent"))
}

func TestGatewayService_BuildUpstreamRequest_RecordsTelemetryPrivacyCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := telemetryPrivacyAnthropicAccount()
	fp := telemetryPrivacyFingerprint()
	cache := &telemetryPrivacyIdentityCacheStub{fingerprint: fp}
	counterRepo := &telemetryPrivacyCounterRepoStub{}
	deferred := NewDeferredService(counterRepo, nil, time.Second)
	svc := &GatewayService{
		identityService: NewIdentityService(cache),
		deferredService: deferred,
	}
	c := telemetryPrivacyGinContext()

	_, err := svc.buildUpstreamRequest(
		context.Background(),
		c,
		account,
		telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
		false,
	)
	require.NoError(t, err)

	deferred.flushTelemetryPrivacyProtectionCounts()
	require.Equal(t, int64(1), counterRepo.counts[account.ID])
}

func TestGatewayService_BuildUpstreamRequest_LogsTelemetryPrivacyWithoutRawIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &telemetryPrivacyLogSinkStub{}
	logger.SetSink(sink)
	t.Cleanup(func() {
		logger.SetSink(nil)
	})

	account := telemetryPrivacyAnthropicAccount()
	fp := telemetryPrivacyFingerprint()
	cache := &telemetryPrivacyIdentityCacheStub{fingerprint: fp}
	svc := &GatewayService{identityService: NewIdentityService(cache)}
	c := telemetryPrivacyGinContext()

	req, err := svc.buildUpstreamRequest(
		context.Background(),
		c,
		account,
		telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
		false,
	)
	require.NoError(t, err)
	require.Len(t, sink.events, 1)

	event := sink.events[0]
	require.Equal(t, "service.gateway.audit.telemetry_privacy", event.Component)
	require.Equal(t, "遥测隐私保护已处理", event.Message)
	require.Equal(t, "messages", event.Fields["endpoint"])
	require.Equal(t, account.ID, event.Fields["account_id"])
	require.Equal(t, true, event.Fields["body_protected"])
	require.Equal(t, true, event.Fields["header_protected"])
	require.Equal(t, true, event.Fields["metadata_user_id_processed"])
	require.Equal(t, "按账号编号派生稳定哈希", event.Fields["metadata_device_id_strategy"])
	require.Equal(t, "按账号编号派生单一稳定会话", event.Fields["metadata_session_id_strategy"])
	require.Equal(t, "强制使用官方客户端默认头指纹", event.Fields["header_fingerprint_strategy"])
	require.Equal(t, false, event.Fields["sensitive_values_logged"])
	_, hasModel := event.Fields["model"]
	require.False(t, hasModel)

	raw, err := json.Marshal(event.Fields)
	require.NoError(t, err)
	text := string(raw)
	require.NotContains(t, text, "claude-3-7-sonnet-20250219")
	require.NotContains(t, text, "oauth-token")
	require.NotContains(t, text, "Bearer oauth-token")
	require.NotContains(t, text, "1111111111111111111111111111111111111111111111111111111111111111")
	require.NotContains(t, text, "550e8400-e29b-41d4-a716-446655440000")
	require.NotContains(t, text, "123e4567-e89b-12d3-a456-426614174000")
	require.NotContains(t, text, "client-request-id-real")
	require.NotContains(t, text, getHeaderRaw(req.Header, "x-client-request-id"))
	require.NotContains(t, text, anthropicTelemetryPrivacyDeviceID(account))
	require.NotContains(t, text, anthropicTelemetryPrivacySessionID(account))

	countReq, err := svc.buildCountTokensRequest(
		context.Background(),
		telemetryPrivacyGinContext(),
		account,
		telemetryPrivacyRequestBody("2222222222222222222222222222222222222222222222222222222222222222", "550e8400-e29b-41d4-a716-446655440000", "223e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
	)
	require.NoError(t, err)
	require.Len(t, sink.events, 2)
	countEvent := sink.events[1]
	require.Equal(t, "count_tokens", countEvent.Fields["endpoint"])
	_, hasCountModel := countEvent.Fields["model"]
	require.False(t, hasCountModel)

	countRaw, err := json.Marshal(countEvent.Fields)
	require.NoError(t, err)
	countText := string(countRaw)
	require.NotContains(t, countText, "claude-3-7-sonnet-20250219")
	require.NotContains(t, countText, "oauth-token")
	require.NotContains(t, countText, "Bearer oauth-token")
	require.NotContains(t, countText, "2222222222222222222222222222222222222222222222222222222222222222")
	require.NotContains(t, countText, "550e8400-e29b-41d4-a716-446655440000")
	require.NotContains(t, countText, "223e4567-e89b-12d3-a456-426614174000")
	require.NotContains(t, countText, "client-request-id-real")
	require.NotContains(t, countText, getHeaderRaw(countReq.Header, "x-client-request-id"))
	require.NotContains(t, countText, anthropicTelemetryPrivacyDeviceID(account))
	require.NotContains(t, countText, anthropicTelemetryPrivacySessionID(account))
}

func TestGatewayService_BuildUpstreamRequest_TelemetryPrivacyOverridesSessionIDMasking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := telemetryPrivacyAnthropicAccount()
	account.Extra["session_id_masking_enabled"] = true
	fp := telemetryPrivacyFingerprint()
	cache := &telemetryPrivacyIdentityCacheStub{
		fingerprint:     fp,
		maskedSessionID: "99999999-9999-4999-8999-999999999999",
	}
	svc := &GatewayService{identityService: NewIdentityService(cache)}
	c := telemetryPrivacyGinContext()

	req, err := svc.buildUpstreamRequest(
		context.Background(),
		c,
		account,
		telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
		false,
	)
	require.NoError(t, err)

	body := telemetryPrivacyReadRequestBody(t, req)
	parsed := ParseMetadataUserID(gjson.GetBytes(body, "metadata.user_id").String())
	require.NotNil(t, parsed)
	require.Equal(t, anthropicTelemetryPrivacySessionID(account), parsed.SessionID)
	require.NotEqual(t, cache.maskedSessionID, parsed.SessionID)
	require.Equal(t, parsed.SessionID, getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"))
}

func TestGatewayService_BuildUpstreamRequest_TelemetryPrivacyOverridesMetadataPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldCache, _ := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings)
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		fingerprintUnification: true,
		metadataPassthrough:    true,
		cchSigning:             false,
		expiresAt:              time.Now().Add(time.Minute).UnixNano(),
	})
	t.Cleanup(func() {
		if oldCache != nil {
			gatewayForwardingCache.Store(oldCache)
			return
		}
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
			fingerprintUnification: true,
			metadataPassthrough:    false,
			cchSigning:             false,
			expiresAt:              0,
		})
	})

	account := telemetryPrivacyAnthropicAccount()
	fp := telemetryPrivacyFingerprint()
	cache := &telemetryPrivacyIdentityCacheStub{fingerprint: fp}
	svc := &GatewayService{
		identityService: NewIdentityService(cache),
		settingService:  NewSettingService(nil, nil),
	}
	c := telemetryPrivacyGinContext()
	c.Set(betaPolicyFilterSetKey, map[string]struct{}{})

	req, err := svc.buildUpstreamRequest(
		context.Background(),
		c,
		account,
		telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
		false,
	)
	require.NoError(t, err)

	body := telemetryPrivacyReadRequestBody(t, req)
	parsed := ParseMetadataUserID(gjson.GetBytes(body, "metadata.user_id").String())
	require.NotNil(t, parsed)
	require.Equal(t, anthropicTelemetryPrivacyDeviceID(account), parsed.DeviceID)
	require.Empty(t, parsed.AccountUUID)
	require.Equal(t, anthropicTelemetryPrivacySessionID(account), parsed.SessionID)
	require.NotContains(t, string(body), "550e8400-e29b-41d4-a716-446655440000")
	require.NotContains(t, string(body), "123e4567-e89b-12d3-a456-426614174000")
}

func TestGatewayService_BuildUpstreamRequest_TelemetryPrivacyOverridesDisabledFingerprintUnification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldCache, _ := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings)
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		fingerprintUnification: false,
		metadataPassthrough:    false,
		cchSigning:             false,
		expiresAt:              time.Now().Add(time.Minute).UnixNano(),
	})
	t.Cleanup(func() {
		if oldCache != nil {
			gatewayForwardingCache.Store(oldCache)
			return
		}
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
			fingerprintUnification: true,
			metadataPassthrough:    false,
			cchSigning:             false,
			expiresAt:              0,
		})
	})

	account := telemetryPrivacyAnthropicAccount()
	fp := telemetryPrivacyFingerprint()
	cache := &telemetryPrivacyIdentityCacheStub{fingerprint: fp}
	svc := &GatewayService{
		identityService: NewIdentityService(cache),
		settingService:  NewSettingService(nil, nil),
	}
	c := telemetryPrivacyGinContext()
	c.Request.Header.Set("User-Agent", "claude-cli/9.9.9 (external, cli)")
	c.Request.Header.Set("X-Stainless-OS", "Windows")
	c.Set(betaPolicyFilterSetKey, map[string]struct{}{})

	req, err := svc.buildUpstreamRequest(
		context.Background(),
		c,
		account,
		telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
		false,
	)
	require.NoError(t, err)

	body := telemetryPrivacyReadRequestBody(t, req)
	parsed := ParseMetadataUserID(gjson.GetBytes(body, "metadata.user_id").String())
	require.NotNil(t, parsed)
	require.Equal(t, anthropicTelemetryPrivacyDeviceID(account), parsed.DeviceID)
	require.Equal(t, anthropicTelemetryPrivacySessionID(account), parsed.SessionID)
	require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, claude.DefaultHeaders["X-Stainless-OS"], getHeaderRaw(req.Header, "X-Stainless-OS"))
	require.Equal(t, claude.DefaultHeaders["X-Stainless-Arch"], getHeaderRaw(req.Header, "X-Stainless-Arch"))
}

func TestGatewayService_BuildCountTokensRequest_AppliesTelemetryPrivacy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := telemetryPrivacyAnthropicAccount()
	fp := telemetryPrivacyFingerprint()
	cache := &telemetryPrivacyIdentityCacheStub{fingerprint: fp}
	svc := &GatewayService{identityService: NewIdentityService(cache)}
	c := telemetryPrivacyGinContext()

	req, err := svc.buildCountTokensRequest(
		context.Background(),
		c,
		account,
		telemetryPrivacyRequestBody("1111111111111111111111111111111111111111111111111111111111111111", "550e8400-e29b-41d4-a716-446655440000", "123e4567-e89b-12d3-a456-426614174000"),
		"oauth-token",
		"oauth",
		"claude-3-7-sonnet-20250219",
		false,
	)
	require.NoError(t, err)

	body := telemetryPrivacyReadRequestBody(t, req)
	parsed := ParseMetadataUserID(gjson.GetBytes(body, "metadata.user_id").String())
	require.NotNil(t, parsed)
	require.Equal(t, anthropicTelemetryPrivacyDeviceID(account), parsed.DeviceID)
	require.Empty(t, parsed.AccountUUID)
	require.Equal(t, anthropicTelemetryPrivacySessionID(account), parsed.SessionID)
	require.Equal(t, parsed.SessionID, getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"))
	require.NotEqual(t, "client-request-id-real", getHeaderRaw(req.Header, "x-client-request-id"))
	require.NotEmpty(t, getHeaderRaw(req.Header, "x-client-request-id"))
	require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, "Bearer oauth-token", getHeaderRaw(req.Header, "authorization"))
	require.Contains(t, req.URL.Path, "/v1/messages/count_tokens")
}

func telemetryPrivacyAnthropicAccount() *Account {
	return &Account{
		ID:       42,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"account_uuid":              "550e8400-e29b-41d4-a716-446655440000",
			"telemetry_privacy_enabled": true,
		},
	}
}

func telemetryPrivacyFingerprint() *Fingerprint {
	return &Fingerprint{
		ClientID:                "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		UserAgent:               "claude-cli/2.1.92 (external, cli)",
		StainlessLang:           "js",
		StainlessPackageVersion: "0.70.0",
		StainlessOS:             "Linux",
		StainlessArch:           "arm64",
		StainlessRuntime:        "node",
		StainlessRuntimeVersion: "v24.13.0",
	}
}

func telemetryPrivacyGinContext() *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(nil))
	req.Header.Set("User-Agent", "claude-cli/2.1.92 (external, cli)")
	req.Header.Set("X-Stainless-Lang", "js")
	req.Header.Set("X-Stainless-Package-Version", "0.70.0")
	req.Header.Set("X-Stainless-OS", "Linux")
	req.Header.Set("X-Stainless-Arch", "arm64")
	req.Header.Set("X-Stainless-Runtime", "node")
	req.Header.Set("X-Stainless-Runtime-Version", "v24.13.0")
	req.Header.Set("X-Claude-Code-Session-Id", "123e4567-e89b-12d3-a456-426614174000")
	req.Header.Set("x-client-request-id", "client-request-id-real")
	c.Request = req
	return c
}

func telemetryPrivacyRequestBody(deviceID, accountUUID, sessionID string) []byte {
	userID := FormatMetadataUserID(deviceID, accountUUID, sessionID, "2.1.92")
	return []byte(`{"model":"claude-3-7-sonnet-20250219","metadata":{"user_id":` + strconvQuote(userID) + `},"messages":[]}`)
}

func telemetryPrivacyReadRequestBody(t *testing.T, req *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	return body
}
