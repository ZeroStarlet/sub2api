package routes

import (
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r *gin.Engine) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Claude Code 遥测日志（静默丢弃，直接返回 200）。
	// 无需认证即可拦截，确保所有遥测日志请求被丢弃。
	// 全局计数器用于平台级可观测性（因无认证无法归属到具体账号）。
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		n := service.IncrementGlobalEventLoggingDrops()
		if n%100 == 1 {
			slog.Info("telemetry_event_logging_drop_global",
				"total_drops", n,
				"path", c.Request.URL.Path,
			)
		}
		c.Status(http.StatusOK)
	})

	// OAuth CLI 端点无条件静默丢弃（/api/oauth/claude_cli/*）。
	// 这些端点使用 Authorization: Bearer <OAuth token> 而非 sub2api API Key，
	// 因此无法通过 API Key 认证中间件归属到具体账号。
	// 采用与 /api/event_logging/batch 相同的预认证无条件丢弃策略，
	// 确保 OAuth 令牌不会通过代理泄漏。
	// 注意：此策略会阻止通过 sub2api 代理进行 OAuth 初始化设置，
	// 但 sub2api 管理员通常预先配置账号，终端用户不经过代理进行 OAuth 设置。
	dropOAuthCLI := func(c *gin.Context) {
		n := service.IncrementGlobalEventLoggingDrops()
		if n%100 == 1 {
			slog.Info("telemetry_oauth_cli_drop_global",
				"total_drops", n,
				"path", c.Request.URL.Path,
			)
		}
		c.Status(http.StatusOK)
	}
	// 精确路径：/api/oauth/claude_cli（无尾随子路径）
	r.Any("/api/oauth/claude_cli", dropOAuthCLI)
	// 通配子路径：/api/oauth/claude_cli/create_api_key、roles 等
	r.Any("/api/oauth/claude_cli/*subpath", dropOAuthCLI)

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}
