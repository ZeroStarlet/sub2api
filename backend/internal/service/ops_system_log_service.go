package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) ListSystemLogs(ctx context.Context, filter *OpsSystemLogFilter) (*OpsSystemLogList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return &OpsSystemLogList{
			Logs:     []*OpsSystemLog{},
			Total:    0,
			Page:     1,
			PageSize: 50,
		}, nil
	}
	if filter == nil {
		filter = &OpsSystemLogFilter{}
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 50
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}

	result, err := s.opsRepo.ListSystemLogs(ctx, filter)
	if err != nil {
		return nil, infraerrors.InternalServer("OPS_SYSTEM_LOG_LIST_FAILED", "Failed to list system logs").WithCause(err)
	}
	return result, nil
}

func (s *OpsService) GetTelemetryPrivacyStats(ctx context.Context, filter *OpsTelemetryPrivacyStatsFilter) (*OpsTelemetryPrivacyStats, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if filter == nil || filter.AccountID <= 0 {
		return nil, infraerrors.BadRequest("OPS_TELEMETRY_PRIVACY_STATS_INVALID_ACCOUNT", "账号 ID 无效")
	}
	if filter.EndTime.IsZero() {
		filter.EndTime = time.Now().UTC()
	}
	if filter.StartTime.IsZero() {
		filter.StartTime = filter.EndTime.Add(-24 * time.Hour)
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TELEMETRY_PRIVACY_STATS_INVALID_RANGE", "时间范围无效")
	}
	if filter.EndTime.Sub(filter.StartTime) > 30*24*time.Hour {
		return nil, infraerrors.BadRequest("OPS_TELEMETRY_PRIVACY_STATS_RANGE_TOO_LARGE", "时间范围不能超过 30 天")
	}
	if s.opsRepo == nil {
		return &OpsTelemetryPrivacyStats{
			AccountID:  filter.AccountID,
			StartTime:  filter.StartTime.UTC(),
			EndTime:    filter.EndTime.UTC(),
			TimeSeries: []OpsTelemetryPrivacyStatsTimeSeriesPoint{},
		}, nil
	}
	stats, err := s.opsRepo.GetTelemetryPrivacyStats(ctx, filter)
	if err != nil {
		return nil, infraerrors.InternalServer("OPS_TELEMETRY_PRIVACY_STATS_FAILED", "遥测隐私统计加载失败").WithCause(err)
	}
	if stats == nil {
		stats = &OpsTelemetryPrivacyStats{}
	}
	stats.AccountID = filter.AccountID
	stats.StartTime = filter.StartTime.UTC()
	stats.EndTime = filter.EndTime.UTC()

	// 同时加载时序数据，供前端绘制保护量趋势折线图；时序查询失败不阻断主统计返回
	timeSeries, tsErr := s.opsRepo.GetTelemetryPrivacyStatsTimeSeries(ctx, filter)
	if tsErr != nil {
		log.Printf("[ops] 遥测隐私时序数据加载失败 account=%d: %v", filter.AccountID, tsErr)
		stats.TimeSeries = []OpsTelemetryPrivacyStatsTimeSeriesPoint{}
	} else {
		stats.TimeSeries = timeSeries
	}

	return stats, nil
}

func (s *OpsService) CleanupSystemLogs(ctx context.Context, filter *OpsSystemLogCleanupFilter, operatorID int64) (int64, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return 0, err
	}
	if s.opsRepo == nil {
		return 0, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if operatorID <= 0 {
		return 0, infraerrors.BadRequest("OPS_SYSTEM_LOG_CLEANUP_INVALID_OPERATOR", "invalid operator")
	}
	if filter == nil {
		filter = &OpsSystemLogCleanupFilter{}
	}
	if filter.EndTime != nil && filter.StartTime != nil && filter.StartTime.After(*filter.EndTime) {
		return 0, infraerrors.BadRequest("OPS_SYSTEM_LOG_CLEANUP_INVALID_RANGE", "invalid time range")
	}

	deletedRows, err := s.opsRepo.DeleteSystemLogs(ctx, filter)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "requires at least one filter") {
			return 0, infraerrors.BadRequest("OPS_SYSTEM_LOG_CLEANUP_FILTER_REQUIRED", "cleanup requires at least one filter condition")
		}
		return 0, infraerrors.InternalServer("OPS_SYSTEM_LOG_CLEANUP_FAILED", "Failed to cleanup system logs").WithCause(err)
	}

	if auditErr := s.opsRepo.InsertSystemLogCleanupAudit(ctx, &OpsSystemLogCleanupAudit{
		CreatedAt:   time.Now().UTC(),
		OperatorID:  operatorID,
		Conditions:  marshalSystemLogCleanupConditions(filter),
		DeletedRows: deletedRows,
	}); auditErr != nil {
		// 审计失败不影响主流程，避免运维清理被阻塞。
		log.Printf("[OpsSystemLog] cleanup audit failed: %v", auditErr)
	}
	return deletedRows, nil
}

func marshalSystemLogCleanupConditions(filter *OpsSystemLogCleanupFilter) string {
	if filter == nil {
		return "{}"
	}
	payload := map[string]any{
		"level":             strings.TrimSpace(filter.Level),
		"component":         strings.TrimSpace(filter.Component),
		"request_id":        strings.TrimSpace(filter.RequestID),
		"client_request_id": strings.TrimSpace(filter.ClientRequestID),
		"platform":          strings.TrimSpace(filter.Platform),
		"model":             strings.TrimSpace(filter.Model),
		"query":             strings.TrimSpace(filter.Query),
	}
	if filter.UserID != nil {
		payload["user_id"] = *filter.UserID
	}
	if filter.AccountID != nil {
		payload["account_id"] = *filter.AccountID
	}
	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		payload["start_time"] = filter.StartTime.UTC().Format(time.RFC3339Nano)
	}
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		payload["end_time"] = filter.EndTime.UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func (s *OpsService) GetSystemLogSinkHealth() OpsSystemLogSinkHealth {
	if s == nil || s.systemLogSink == nil {
		return OpsSystemLogSinkHealth{}
	}
	return s.systemLogSink.Health()
}
