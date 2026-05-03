package service

import "time"

type OpsSystemLog struct {
	ID              int64          `json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	Level           string         `json:"level"`
	Component       string         `json:"component"`
	Message         string         `json:"message"`
	RequestID       string         `json:"request_id"`
	ClientRequestID string         `json:"client_request_id"`
	UserID          *int64         `json:"user_id"`
	AccountID       *int64         `json:"account_id"`
	Platform        string         `json:"platform"`
	Model           string         `json:"model"`
	Extra           map[string]any `json:"extra,omitempty"`
}

type OpsTelemetryPrivacyStatsBreakdownItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type OpsTelemetryPrivacyStats struct {
	AccountID int64     `json:"account_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Total                             int64 `json:"total"`
	SuccessCount                      int64 `json:"success_count"`
	FailureCount                      int64 `json:"failure_count"`
	BodyProtectedCount                int64 `json:"body_protected_count"`
	BodyPassCount                     int64 `json:"body_pass_count"`
	BodyRewrittenCount                int64 `json:"body_rewritten_count"`
	MetadataPresentCount              int64 `json:"metadata_present_count"`
	MetadataAbsentSafeCount           int64 `json:"metadata_absent_safe_count"`
	HeaderProtectedCount              int64 `json:"header_protected_count"`
	HeaderPassCount                   int64 `json:"header_pass_count"`
	HeaderFingerprintDefaultCount     int64 `json:"header_fingerprint_default_count"`
	UserAgentDefaultCount             int64 `json:"user_agent_default_count"`
	XStainlessDefaultCount            int64 `json:"x_stainless_default_count"`
	XAppDefaultCount                  int64 `json:"x_app_default_count"`
	DirectBrowserAccessDefaultCount   int64 `json:"direct_browser_access_default_count"`
	TLSPassCount                      int64 `json:"tls_pass_count"`
	TLSDefaultCount                   int64 `json:"tls_default_count"`
	ClientRequestIDResetCount         int64 `json:"client_request_id_reset_count"`
	SessionHeaderProtectedCount       int64 `json:"session_header_protected_count"`
	RawValuesLoggedCount              int64 `json:"raw_values_logged_count"`
	DerivedValuesLoggedCount          int64 `json:"derived_values_logged_count"`
	AuthorizationValueLoggedCount     int64 `json:"authorization_value_logged_count"`
	TokenValueLoggedCount             int64 `json:"token_value_logged_count"`
	ModelValueLoggedCount             int64 `json:"model_value_logged_count"`
	RequestBodyLoggedCount            int64 `json:"request_body_logged_count"`
	UniqueRawDeviceIDCount            int64 `json:"unique_raw_device_id_count"`
	UniqueRawSessionIDCount           int64 `json:"unique_raw_session_id_count"`
	UniqueRawClientRequestIDCount     int64 `json:"unique_raw_client_request_id_count"`
	UniqueDerivedDeviceIDCount        int64 `json:"unique_derived_device_id_count"`
	UniqueDerivedSessionIDCount       int64 `json:"unique_derived_session_id_count"`
	UniqueDerivedClientRequestIDCount int64 `json:"unique_derived_client_request_id_count"`

	EndpointBreakdown []OpsTelemetryPrivacyStatsBreakdownItem `json:"endpoint_breakdown"`
	ResultBreakdown   []OpsTelemetryPrivacyStatsBreakdownItem `json:"result_breakdown"`
}

type OpsErrorLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	// Standardized classification
	// - phase: request|auth|routing|upstream|network|internal
	// - owner: client|provider|platform
	// - source: client_request|upstream_http|gateway
	Phase string `json:"phase"`
	Type  string `json:"type"`

	Owner  string `json:"error_owner"`
	Source string `json:"error_source"`

	Severity string `json:"severity"`

	StatusCode int    `json:"status_code"`
	Platform   string `json:"platform"`
	Model      string `json:"model"`

	IsRetryable bool `json:"is_retryable"`
	RetryCount  int  `json:"retry_count"`

	Resolved           bool       `json:"resolved"`
	ResolvedAt         *time.Time `json:"resolved_at"`
	ResolvedByUserID   *int64     `json:"resolved_by_user_id"`
	ResolvedByUserName string     `json:"resolved_by_user_name"`
	ResolvedRetryID    *int64     `json:"resolved_retry_id"`
	ResolvedStatusRaw  string     `json:"-"`

	ClientRequestID string `json:"client_request_id"`
	RequestID       string `json:"request_id"`
	Message         string `json:"message"`

	UserID      *int64 `json:"user_id"`
	UserEmail   string `json:"user_email"`
	APIKeyID    *int64 `json:"api_key_id"`
	AccountID   *int64 `json:"account_id"`
	AccountName string `json:"account_name"`
	GroupID     *int64 `json:"group_id"`
	GroupName   string `json:"group_name"`

	ClientIP    *string `json:"client_ip"`
	RequestPath string  `json:"request_path"`
	Stream      bool    `json:"stream"`

	InboundEndpoint  string `json:"inbound_endpoint"`
	UpstreamEndpoint string `json:"upstream_endpoint"`
	RequestedModel   string `json:"requested_model"`
	UpstreamModel    string `json:"upstream_model"`
	RequestType      *int16 `json:"request_type"`
}

type OpsErrorLogDetail struct {
	OpsErrorLog

	ErrorBody string `json:"error_body"`
	UserAgent string `json:"user_agent"`

	// Upstream context (optional)
	UpstreamStatusCode   *int   `json:"upstream_status_code,omitempty"`
	UpstreamErrorMessage string `json:"upstream_error_message,omitempty"`
	UpstreamErrorDetail  string `json:"upstream_error_detail,omitempty"`
	UpstreamErrors       string `json:"upstream_errors,omitempty"` // JSON array (string) for display/parsing

	// Timings (optional)
	AuthLatencyMs      *int64 `json:"auth_latency_ms"`
	RoutingLatencyMs   *int64 `json:"routing_latency_ms"`
	UpstreamLatencyMs  *int64 `json:"upstream_latency_ms"`
	ResponseLatencyMs  *int64 `json:"response_latency_ms"`
	TimeToFirstTokenMs *int64 `json:"time_to_first_token_ms"`

	// Retry context
	RequestBody          string `json:"request_body"`
	RequestBodyTruncated bool   `json:"request_body_truncated"`
	RequestBodyBytes     *int   `json:"request_body_bytes"`
	RequestHeaders       string `json:"request_headers,omitempty"`

	// vNext metric semantics
	IsBusinessLimited bool `json:"is_business_limited"`
}

type OpsErrorLogFilter struct {
	StartTime *time.Time
	EndTime   *time.Time

	Platform  string
	GroupID   *int64
	AccountID *int64

	StatusCodes      []int
	StatusCodesOther bool
	Phase            string
	Owner            string
	Source           string
	Resolved         *bool
	Query            string
	UserQuery        string // Search by user email

	// Optional correlation keys for exact matching.
	RequestID       string
	ClientRequestID string

	// View controls error categorization for list endpoints.
	// - errors: show actionable errors (exclude business-limited / 429 / 529)
	// - excluded: only show excluded errors
	// - all: show everything
	View string

	Page     int
	PageSize int
}

type OpsErrorLogList struct {
	Errors   []*OpsErrorLog `json:"errors"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

type OpsRetryAttempt struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	RequestedByUserID int64  `json:"requested_by_user_id"`
	SourceErrorID     int64  `json:"source_error_id"`
	Mode              string `json:"mode"`
	PinnedAccountID   *int64 `json:"pinned_account_id"`
	PinnedAccountName string `json:"pinned_account_name"`

	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMs *int64     `json:"duration_ms"`

	// Persisted execution results (best-effort)
	Success           *bool   `json:"success"`
	HTTPStatusCode    *int    `json:"http_status_code"`
	UpstreamRequestID *string `json:"upstream_request_id"`
	UsedAccountID     *int64  `json:"used_account_id"`
	UsedAccountName   string  `json:"used_account_name"`
	ResponsePreview   *string `json:"response_preview"`
	ResponseTruncated *bool   `json:"response_truncated"`

	// Optional correlation
	ResultRequestID *string `json:"result_request_id"`
	ResultErrorID   *int64  `json:"result_error_id"`

	ErrorMessage *string `json:"error_message"`
}

type OpsRetryResult struct {
	AttemptID int64  `json:"attempt_id"`
	Mode      string `json:"mode"`
	Status    string `json:"status"`

	PinnedAccountID *int64 `json:"pinned_account_id"`
	UsedAccountID   *int64 `json:"used_account_id"`

	HTTPStatusCode    int    `json:"http_status_code"`
	UpstreamRequestID string `json:"upstream_request_id"`

	ResponsePreview   string `json:"response_preview"`
	ResponseTruncated bool   `json:"response_truncated"`

	ErrorMessage string `json:"error_message"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMs int64     `json:"duration_ms"`
}
