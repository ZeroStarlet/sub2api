package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildOpsSystemLogsWhere_WithClientRequestIDAndUserID(t *testing.T) {
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	userID := int64(12)
	accountID := int64(34)

	filter := &service.OpsSystemLogFilter{
		StartTime:       &start,
		EndTime:         &end,
		Level:           "warn",
		Component:       "http.access",
		RequestID:       "req-1",
		ClientRequestID: "creq-1",
		UserID:          &userID,
		AccountID:       &accountID,
		Platform:        "openai",
		Model:           "gpt-5",
		Query:           "timeout",
	}

	where, args, hasConstraint := buildOpsSystemLogsWhere(filter)
	if !hasConstraint {
		t.Fatalf("expected hasConstraint=true")
	}
	if where == "" {
		t.Fatalf("where should not be empty")
	}
	if len(args) != 11 {
		t.Fatalf("args len = %d, want 11", len(args))
	}
	if !contains(where, "COALESCE(l.client_request_id,'') = $") {
		t.Fatalf("where should include client_request_id condition: %s", where)
	}
	if !contains(where, "l.user_id = $") {
		t.Fatalf("where should include user_id condition: %s", where)
	}
}

func TestBuildOpsSystemLogsCleanupWhere_RequireConstraint(t *testing.T) {
	where, args, hasConstraint := buildOpsSystemLogsCleanupWhere(&service.OpsSystemLogCleanupFilter{})
	if hasConstraint {
		t.Fatalf("expected hasConstraint=false")
	}
	if where == "" {
		t.Fatalf("where should not be empty")
	}
	if len(args) != 0 {
		t.Fatalf("args len = %d, want 0", len(args))
	}
}

func TestBuildOpsSystemLogsCleanupWhere_WithClientRequestIDAndUserID(t *testing.T) {
	userID := int64(9)
	filter := &service.OpsSystemLogCleanupFilter{
		ClientRequestID: "creq-9",
		UserID:          &userID,
	}

	where, args, hasConstraint := buildOpsSystemLogsCleanupWhere(filter)
	if !hasConstraint {
		t.Fatalf("expected hasConstraint=true")
	}
	if len(args) != 2 {
		t.Fatalf("args len = %d, want 2", len(args))
	}
	if !contains(where, "COALESCE(l.client_request_id,'') = $") {
		t.Fatalf("where should include client_request_id condition: %s", where)
	}
	if !contains(where, "l.user_id = $") {
		t.Fatalf("where should include user_id condition: %s", where)
	}
}

func TestTelemetryPrivacyStatsBreakdownLabel(t *testing.T) {
	if got := telemetryPrivacyStatsBreakdownLabel("endpoint", "messages"); got != "消息创建" {
		t.Fatalf("messages label=%q", got)
	}
	if got := telemetryPrivacyStatsBreakdownLabel("endpoint", "count_tokens"); got != "令牌计数" {
		t.Fatalf("count_tokens label=%q", got)
	}
	if got := telemetryPrivacyStatsBreakdownLabel("body_result", "metadata.user_id 已替换为账号级匿名遥测身份"); got != "metadata.user_id 已替换为账号级匿名遥测身份" {
		t.Fatalf("body_result label=%q", got)
	}
	if got := telemetryPrivacyStatsBreakdownLabel("body_result", `raw_device_id=raw-device-001`); got != "其他处理结果" {
		t.Fatalf("unsafe body_result label=%q", got)
	}
	if got := telemetryPrivacyStatsBreakdownLabel("endpoint", ""); got != "-" {
		t.Fatalf("empty label=%q", got)
	}
}

func TestOpsRepositoryGetTelemetryPrivacyStats_AggregatesOnly(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	aggregateRows := sqlmock.NewRows([]string{
		"total",
		"success",
		"failure",
		"body_protected",
		"body_pass",
		"body_rewritten",
		"metadata_present",
		"metadata_absent_safe",
		"header_protected",
		"header_pass",
		"header_default",
		"user_agent_default",
		"x_stainless_default",
		"x_app_default",
		"direct_browser_access_default",
		"tls_pass",
		"tls_default",
		"request_id_reset",
		"session_header_protected",
		"raw_values_logged",
		"derived_values_logged",
		"authorization_logged",
		"token_logged",
		"model_logged",
		"request_body_logged",
		"unique_raw_device",
		"unique_raw_session",
		"unique_raw_request",
		"unique_derived_device",
		"unique_derived_session",
		"unique_derived_request",
	}).AddRow(
		int64(13),
		int64(12),
		int64(1),
		int64(13),
		int64(12),
		int64(12),
		int64(13),
		int64(0),
		int64(13),
		int64(12),
		int64(13),
		int64(13),
		int64(13),
		int64(13),
		int64(13),
		int64(13),
		int64(13),
		int64(13),
		int64(13),
		int64(13),
		int64(13),
		int64(0),
		int64(0),
		int64(0),
		int64(0),
		int64(9),
		int64(9),
		int64(13),
		int64(1),
		int64(1),
		int64(13),
	)
	mock.ExpectQuery("SELECT\\s+COUNT\\(\\*\\)::bigint").
		WithArgs(int64(4), start, end).
		WillReturnRows(aggregateRows)
	mock.ExpectQuery("SELECT COALESCE.*GROUP BY k").
		WithArgs(int64(4), start, end, "endpoint").
		WillReturnRows(sqlmock.NewRows([]string{"k", "count"}).AddRow("messages", int64(13)))
	mock.ExpectQuery("SELECT COALESCE.*GROUP BY k").
		WithArgs(int64(4), start, end, "body_result").
		WillReturnRows(sqlmock.NewRows([]string{"k", "count"}).
			AddRow("metadata.user_id 已替换为账号级匿名遥测身份", int64(12)).
			AddRow("raw_device_id=raw-device-001", int64(1)))

	stats, err := repo.GetTelemetryPrivacyStats(context.Background(), &service.OpsTelemetryPrivacyStatsFilter{
		AccountID: 4,
		StartTime: start,
		EndTime:   end,
	})
	if err != nil {
		t.Fatalf("GetTelemetryPrivacyStats() error: %v", err)
	}
	if stats.Total != 13 || stats.SuccessCount != 12 || stats.FailureCount != 1 {
		t.Fatalf("unexpected totals: %+v", stats)
	}
	if stats.UniqueRawDeviceIDCount != 9 || stats.UniqueDerivedDeviceIDCount != 1 {
		t.Fatalf("unexpected identity counts: %+v", stats)
	}
	if len(stats.EndpointBreakdown) != 1 || stats.EndpointBreakdown[0].Label != "消息创建" {
		t.Fatalf("unexpected endpoint breakdown: %+v", stats.EndpointBreakdown)
	}
	if len(stats.ResultBreakdown) != 2 || stats.ResultBreakdown[0].Count != 12 || stats.ResultBreakdown[1].Label != "其他处理结果" || stats.ResultBreakdown[1].Count != 1 {
		t.Fatalf("unexpected result breakdown: %+v", stats.ResultBreakdown)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestOpsRepositoryGetTelemetryPrivacyStats_EmptyBreakdowns(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	aggregateRows := sqlmock.NewRows([]string{
		"total",
		"success",
		"failure",
		"body_protected",
		"body_pass",
		"body_rewritten",
		"metadata_present",
		"metadata_absent_safe",
		"header_protected",
		"header_pass",
		"header_default",
		"user_agent_default",
		"x_stainless_default",
		"x_app_default",
		"direct_browser_access_default",
		"tls_pass",
		"tls_default",
		"request_id_reset",
		"session_header_protected",
		"raw_values_logged",
		"derived_values_logged",
		"authorization_logged",
		"token_logged",
		"model_logged",
		"request_body_logged",
		"unique_raw_device",
		"unique_raw_session",
		"unique_raw_request",
		"unique_derived_device",
		"unique_derived_session",
		"unique_derived_request",
	}).AddRow(
		int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0),
		int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0),
		int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0),
		int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0),
	)
	mock.ExpectQuery("SELECT\\s+COUNT\\(\\*\\)::bigint").
		WithArgs(int64(8), start, end).
		WillReturnRows(aggregateRows)
	mock.ExpectQuery("SELECT COALESCE.*GROUP BY k").
		WithArgs(int64(8), start, end, "endpoint").
		WillReturnRows(sqlmock.NewRows([]string{"k", "count"}))
	mock.ExpectQuery("SELECT COALESCE.*GROUP BY k").
		WithArgs(int64(8), start, end, "body_result").
		WillReturnRows(sqlmock.NewRows([]string{"k", "count"}))

	stats, err := repo.GetTelemetryPrivacyStats(context.Background(), &service.OpsTelemetryPrivacyStatsFilter{
		AccountID: 8,
		StartTime: start,
		EndTime:   end,
	})
	if err != nil {
		t.Fatalf("GetTelemetryPrivacyStats() error: %v", err)
	}
	if stats.Total != 0 || len(stats.EndpointBreakdown) != 0 || len(stats.ResultBreakdown) != 0 {
		t.Fatalf("unexpected empty stats: %+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func contains(s string, sub string) bool {
	return strings.Contains(s, sub)
}
