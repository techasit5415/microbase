package routes

import (
	"context"
	"strconv"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/config"
	"github.com/techasit5415/microbase/internal/controllers"
	"github.com/techasit5415/microbase/internal/hooks"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/realtime"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/storage"
	"github.com/techasit5415/microbase/ui"
)

// Register attaches global middlewares, embedded UI, and API routes to Echo.
func Register(e *echo.Echo, pool *pgxpool.Pool, cacheStore *cache.SwitchableStore, cfg *config.Config) {
	e.Use(middleware.Recover())
	if cfg != nil && strings.TrimSpace(cfg.Origins) != "" {
		allowed := make([]string, 0)
		for _, o := range strings.Split(cfg.Origins, ",") {
			if s := strings.TrimSpace(o); s != "" {
				allowed = append(allowed, s)
			}
		}
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{AllowOrigins: allowed}))
	} else {
		e.Use(middleware.CORS())
	}
	e.Use(middlewares.RequestLogger())

	// Embedded SvelteKit Admin Dashboard under /_/ (skipped when --no-ui / DISABLE_ADMIN_UI=true).
	if cfg == nil || !cfg.DisableAdminUI {
		ui.Register(e)
	}

	// Health endpoint.
	healthCtrl := controllers.NewHealthController(cacheStore)
	e.GET("/health", healthCtrl.Check)

	if pool != nil {
		middlewares.SetLogPool(pool)
		services.SetAuthOriginPool(pool)
		services.LoadAppSettingsFromDB(context.Background(), pool)
		middlewares.LoadRateLimitConfigFromDB(context.Background(), pool)
		middlewares.LoadLogSettingsFromDB(context.Background(), pool)
		services.StartCronScheduler(pool)
		hub := realtime.NewHub()
		webhookService := services.NewWebhookService(pool)
		colService := services.NewCollectionService(pool, cacheStore)
		recService := services.NewRecordService(pool, colService, hub, webhookService)
		hooksDir := "./mb_hooks"
		if cfg != nil && cfg.HooksDir != "" {
			hooksDir = cfg.HooksDir
		}
		hookEngine := hooks.NewEngine(pool)
		if err := hookEngine.LoadDir(hooksDir); err != nil {
			slog.Error("failed to load js hooks", "dir", hooksDir, "error", err)
		} else {
			recService.SetHooks(hookEngine)
			hookEngine.RegisterRoutes(e)
		}
		authService := services.NewAuthService(pool, cfg.JWTSecret)
		superService := services.NewSuperuserService(pool, authService, cacheStore)
		oauthService := services.NewOAuthService(pool, cacheStore, authService)

		colCtrl := controllers.NewCollectionController(colService)
		schemaCtrl := controllers.NewSchemaController(colService)
		recCtrl := controllers.NewRecordController(recService, colService)
		colAuthCtrl := controllers.NewCollectionAuthController(pool, colService, authService)
		authCtrl := controllers.NewAuthController(authService, colService)
		authExtraCtrl := controllers.NewAuthExtraController(pool, authService, cacheStore)
		superCtrl := controllers.NewSuperuserController(superService)
		oauthCtrl := controllers.NewOAuthController(oauthService)
		realtimeCtrl := controllers.NewRealtimeController(hub, recService, colService)
		var savedS3 *storage.S3Config
		var rawS3 string
		if err := pool.QueryRow(context.Background(), `SELECT value FROM _settings WHERE key = 's3_config'`).Scan(&rawS3); err == nil && rawS3 != "" {
			var cfg storage.S3Config
			if json.Unmarshal([]byte(rawS3), &cfg) == nil {
				savedS3 = &cfg
			}
		}
		storageDir := "./storage"
		if cfg != nil && cfg.DataDir != "" {
			storageDir = cfg.DataDir
		}
		fileStorage := storage.NewSwitchableStorage(storageDir, savedS3)
		fileCtrl := controllers.NewFileController(fileStorage, authService)
		systemCtrl := controllers.NewSystemController(pool)
		webhookCtrl := controllers.NewWebhookController(webhookService)
		openapiCtrl := controllers.NewOpenAPIController(colService)
		settingsCtrl := controllers.NewSettingsController(pool, cacheStore, fileStorage)
		toolsCtrl := controllers.NewAdminToolsController(pool)
		opsCtrl := controllers.NewOpsController(pool, "./backups")

		api := e.Group("/api", middlewares.RateLimit(cacheStore, 100, time.Minute), middlewares.OptionalAuth(authService))

		adminOnly := middlewares.RequireSuperuser(authService)

		// OpenAPI 3.0 Spec & Batch.
		api.GET("/openapi.json", openapiCtrl.Spec)
		api.GET("/settings/app", opsCtrl.GetAppSettings)
		api.PUT("/settings/app", opsCtrl.UpdateAppSettings, adminOnly)
		api.POST("/batch", toolsCtrl.Batch, middlewares.OptionalAuth(authService))

		// Superuser-only Settings, Backups, Crons, SQL.
		api.GET("/settings/redis", settingsCtrl.GetRedis, adminOnly)
		api.PUT("/settings/redis", settingsCtrl.UpdateRedis, adminOnly)
		api.GET("/settings/s3", settingsCtrl.GetS3, adminOnly)
		api.PUT("/settings/s3", settingsCtrl.UpdateS3, adminOnly)
		api.POST("/sql", toolsCtrl.ExecuteSQL, adminOnly)
		api.GET("/settings/mail", opsCtrl.GetMail, adminOnly)
		api.PUT("/settings/mail", opsCtrl.UpdateMail, adminOnly)
		api.POST("/settings/mail/test", opsCtrl.TestMail, adminOnly)
		api.GET("/backups", opsCtrl.ListBackups, adminOnly)
		api.POST("/backups", opsCtrl.CreateBackup, adminOnly)
		api.GET("/backups/:name", opsCtrl.DownloadBackup, adminOnly)
		api.DELETE("/backups/:name", opsCtrl.DeleteBackup, adminOnly)
		api.POST("/backups/upload", opsCtrl.UploadBackup, adminOnly)
		api.POST("/backups/:name/restore", opsCtrl.RestoreBackup, adminOnly)
		api.GET("/crons", opsCtrl.ListCrons, adminOnly)
		api.POST("/crons", opsCtrl.CreateCron, adminOnly)
		api.PATCH("/crons/:id", opsCtrl.UpdateCron, adminOnly)
		api.DELETE("/crons/:id", opsCtrl.DeleteCron, adminOnly)
		api.POST("/crons/:id/run", opsCtrl.RunCron, adminOnly)
		api.GET("/settings/ratelimits", opsCtrl.GetRateLimits, adminOnly)
		api.PUT("/settings/ratelimits", opsCtrl.UpdateRateLimits, adminOnly)
		api.GET("/settings/logs", opsCtrl.GetLogSettings, adminOnly)
		api.PUT("/settings/logs", opsCtrl.UpdateLogSettings, adminOnly)
		api.GET("/settings/database", opsCtrl.GetDatabase, adminOnly)
		api.POST("/settings/database/test", opsCtrl.TestDatabase, adminOnly)
		api.PUT("/settings/database", opsCtrl.UpdateDatabase, adminOnly)

		// Auth, Superusers & OAuth2/OIDC endpoints.
		api.GET("/auth/methods", oauthCtrl.AuthMethods)
		api.POST("/auth/register", authCtrl.Register, middlewares.OptionalAuth(authService))
		api.POST("/auth/login", authCtrl.Login)
		api.GET("/auth/me", authCtrl.Me, middlewares.RequireAuth(authService))
		api.POST("/auth/refresh", authExtraCtrl.Refresh, middlewares.RequireAuth(authService))
		api.POST("/auth/request-verification", authExtraCtrl.RequestVerification)
		api.POST("/auth/confirm-verification", authExtraCtrl.ConfirmVerification)
		api.POST("/auth/request-password-reset", authExtraCtrl.RequestPasswordReset)
		api.POST("/auth/confirm-password-reset", authExtraCtrl.ConfirmPasswordReset)
		api.POST("/auth/request-email-change", authExtraCtrl.RequestEmailChange, middlewares.RequireAuth(authService))
		api.POST("/auth/confirm-email-change", authExtraCtrl.ConfirmEmailChange)
		api.POST("/auth/request-otp", authExtraCtrl.RequestOTP)
		api.POST("/auth/auth-with-otp", authExtraCtrl.AuthWithOTP)
		api.POST("/users/:id/impersonate", authExtraCtrl.Impersonate, adminOnly)
		api.GET("/settings/auth-options", authExtraCtrl.GetAuthOptions, adminOnly)
		api.PUT("/settings/auth-options", authExtraCtrl.UpdateAuthOptions, adminOnly)
		usersGroup := api.Group("/users", middlewares.OptionalAuth(authService))
		usersGroup.GET("", authCtrl.ListUsers)
		usersGroup.GET("/:id", authCtrl.GetUser)
		usersGroup.PATCH("/:id", authCtrl.UpdateUser)
		usersGroup.DELETE("/:id", authCtrl.DeleteUser)
		api.GET("/superusers/status", superCtrl.Status)
		api.POST("/superusers/setup", superCtrl.Setup)
		api.POST("/superusers/login", superCtrl.Login)
		api.POST("/superusers", superCtrl.Create, adminOnly)
		api.PATCH("/superusers/:id", superCtrl.Update, adminOnly)
		api.DELETE("/superusers/:id", superCtrl.Delete, adminOnly)
		api.GET("/auth/oauth/:provider", oauthCtrl.Start)
		api.GET("/auth/oauth/:provider/callback", oauthCtrl.Callback)
		api.POST("/auth/oauth/exchange", oauthCtrl.Exchange)

		// OAuth2/OIDC Provider Admin Configuration.
		api.GET("/oauth/providers", oauthCtrl.ListProviders, adminOnly)
		api.PUT("/oauth/providers/:name", oauthCtrl.UpdateProvider, adminOnly)

		// Webhooks, Realtime SSE, File Storage, System Tables, Logs.
		api.GET("/webhooks", webhookCtrl.List, adminOnly)
		api.POST("/webhooks", webhookCtrl.Create, adminOnly)
		api.PATCH("/webhooks/:id", webhookCtrl.Toggle, adminOnly)
		api.POST("/webhooks/:id/test", webhookCtrl.Test, adminOnly)
		api.DELETE("/webhooks/:id", webhookCtrl.Delete, adminOnly)
		api.GET("/realtime", realtimeCtrl.Stream, middlewares.OptionalAuth(authService))
		api.POST("/files", fileCtrl.Upload)
		api.POST("/files/token", fileCtrl.CreateToken)
		api.GET("/files/:filename", fileCtrl.Serve)
		api.GET("/system/:table", systemCtrl.List, adminOnly)
		api.POST("/system/:table", systemCtrl.Create, adminOnly)
		api.PATCH("/system/:table/:id", systemCtrl.Update, adminOnly)
		api.DELETE("/system/:table/:id", systemCtrl.Delete, adminOnly)
		api.GET("/logs", func(c echo.Context) error {
			page, _ := strconv.Atoi(c.QueryParam("page"))
			if page < 1 {
				page = 1
			}
			perPageStr := c.QueryParam("per_page")
			if perPageStr == "" {
				perPageStr = c.QueryParam("perPage")
			}
			perPage, _ := strconv.Atoi(perPageStr)
			if perPage < 1 {
				perPage = 30
			}
			if perPage > 200 {
				perPage = 200
			}
			search := c.QueryParam("search")
			if search == "" {
				search = c.QueryParam("q")
			}
			items, total := middlewares.QueryLogs(c.Request().Context(), page, perPage, search)
			totalPages := int((total + int64(perPage) - 1) / int64(perPage))
			if totalPages < 1 {
				totalPages = 1
			}
			return c.JSON(200, map[string]any{
				"page":        page,
				"per_page":    perPage,
				"total_items": total,
				"total_pages": totalPages,
				"items":       items,
			})
		}, adminOnly)
		api.DELETE("/logs", opsCtrl.ClearLogs, adminOnly)

		// Collections & Schema Portability.
		api.GET("/collections", colCtrl.List, adminOnly)
		api.POST("/collections", colCtrl.Create, adminOnly)
		api.GET("/collections/:name", colCtrl.Get, adminOnly)
		api.PATCH("/collections/:name", colCtrl.Update, adminOnly)
		api.DELETE("/collections/:name", colCtrl.Delete, adminOnly)
		api.POST("/collections/:name/truncate", colCtrl.Truncate, adminOnly)
		api.POST("/collections/:name/duplicate", colCtrl.Duplicate, adminOnly)
		api.GET("/schema/export", schemaCtrl.Export, adminOnly)
		api.POST("/schema/import", schemaCtrl.Import, adminOnly)

		// Dynamic Records & Search (with OptionalAuth for API Rules enforcement).
		recGroup := api.Group("/collections/:name", middlewares.OptionalAuth(authService))
		recGroup.GET("/records", recCtrl.List)
		recGroup.POST("/records", recCtrl.Create)
		recGroup.GET("/records/:id", recCtrl.Get)
		recGroup.PATCH("/records/:id", recCtrl.Update)
		recGroup.DELETE("/records/:id", recCtrl.Delete)
		recGroup.POST("/search/vector", recCtrl.VectorSearch)
		recGroup.POST("/register", colAuthCtrl.Register)
		recGroup.POST("/auth-with-password", colAuthCtrl.AuthWithPassword)
		recGroup.POST("/auth-refresh", colAuthCtrl.AuthRefresh)
	} else {
		e.GET("/api/superusers/status", func(c echo.Context) error {
			return response.OK(c, map[string]any{
				"has_superuser":      false,
				"database_connected": false,
				"message":            "PostgreSQL is not connected. Set DATABASE_URL in .env or pass --db and restart Microbase.",
			})
		})
		e.Any("/api/*", func(c echo.Context) error {
			return response.Error(c, http.StatusServiceUnavailable, "DATABASE_NOT_CONFIGURED",
				"PostgreSQL is not connected. Set DATABASE_URL in .env or pass --db and restart Microbase.")
		})
	}
}
