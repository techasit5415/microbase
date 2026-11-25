package routes

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/config"
	"github.com/techasit5415/microbase/internal/controllers"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/realtime"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/storage"
	"github.com/techasit5415/microbase/ui"
)

// Register attaches global middlewares, embedded UI, and API routes to Echo.
func Register(e *echo.Echo, pool *pgxpool.Pool, cacheStore *cache.SwitchableStore, cfg *config.Config) {
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middlewares.RequestLogger())

	// Embedded SvelteKit Admin Dashboard under /_/.
	ui.Register(e)

	// Health endpoint.
	healthCtrl := controllers.NewHealthController(cacheStore)
	e.GET("/health", healthCtrl.Check)

	if pool != nil {
		middlewares.SetLogPool(pool)
		hub := realtime.NewHub()
		webhookService := services.NewWebhookService(pool)
		colService := services.NewCollectionService(pool, cacheStore)
		recService := services.NewRecordService(pool, colService, hub, webhookService)
		authService := services.NewAuthService(pool, cfg.JWTSecret)
		superService := services.NewSuperuserService(pool, authService, cacheStore)
		oauthService := services.NewOAuthService(pool, cacheStore, authService)

		colCtrl := controllers.NewCollectionController(colService)
		schemaCtrl := controllers.NewSchemaController(colService)
		recCtrl := controllers.NewRecordController(recService, colService)
		authCtrl := controllers.NewAuthController(authService)
		superCtrl := controllers.NewSuperuserController(superService)
		oauthCtrl := controllers.NewOAuthController(oauthService)
		realtimeCtrl := controllers.NewRealtimeController(hub)
		var savedS3 *storage.S3Config
		var rawS3 string
		if err := pool.QueryRow(context.Background(), `SELECT value FROM _settings WHERE key = 's3_config'`).Scan(&rawS3); err == nil && rawS3 != "" {
			var cfg storage.S3Config
			if json.Unmarshal([]byte(rawS3), &cfg) == nil {
				savedS3 = &cfg
			}
		}
		fileStorage := storage.NewSwitchableStorage("./storage", savedS3)
		fileCtrl := controllers.NewFileController(fileStorage)
		systemCtrl := controllers.NewSystemController(pool)
		webhookCtrl := controllers.NewWebhookController(webhookService)
		openapiCtrl := controllers.NewOpenAPIController(colService)
		settingsCtrl := controllers.NewSettingsController(pool, cacheStore, fileStorage)

		api := e.Group("/api", middlewares.RateLimit(cacheStore, 100, time.Minute))

		// OpenAPI 3.0 Spec.
		api.GET("/openapi.json", openapiCtrl.Spec)
		api.GET("/settings/redis", settingsCtrl.GetRedis)
		api.PUT("/settings/redis", settingsCtrl.UpdateRedis)
		api.GET("/settings/s3", settingsCtrl.GetS3)
		api.PUT("/settings/s3", settingsCtrl.UpdateS3)

		// Auth, Superusers & OAuth2/OIDC endpoints.
		api.GET("/auth/methods", oauthCtrl.AuthMethods)
		api.POST("/auth/register", authCtrl.Register)
		api.POST("/auth/login", authCtrl.Login)
		api.GET("/auth/me", authCtrl.Me, middlewares.RequireAuth(authService))
		api.GET("/users", authCtrl.ListUsers)
		api.DELETE("/users/:id", authCtrl.DeleteUser)
		api.GET("/superusers/status", superCtrl.Status)
		api.POST("/superusers/setup", superCtrl.Setup)
		api.POST("/superusers/login", superCtrl.Login)
		api.POST("/superusers", superCtrl.Create, middlewares.RequireAuth(authService))
		api.DELETE("/superusers/:id", superCtrl.Delete, middlewares.RequireAuth(authService))
		api.GET("/auth/oauth/:provider", oauthCtrl.Start)
		api.GET("/auth/oauth/:provider/callback", oauthCtrl.Callback)
		api.POST("/auth/oauth/exchange", oauthCtrl.Exchange)

		// OAuth2/OIDC Provider Admin Configuration.
		api.GET("/oauth/providers", oauthCtrl.ListProviders)
		api.PUT("/oauth/providers/:name", oauthCtrl.UpdateProvider)

		// Webhooks, Realtime SSE, File Storage, System Tables, Logs.
		api.GET("/webhooks", webhookCtrl.List)
		api.POST("/webhooks", webhookCtrl.Create)
		api.DELETE("/webhooks/:id", webhookCtrl.Delete)
		api.GET("/realtime", realtimeCtrl.Stream)
		api.POST("/files", fileCtrl.Upload)
		api.GET("/files/:filename", fileCtrl.Serve)
		api.GET("/system/:table", systemCtrl.List)
		api.GET("/logs", func(c echo.Context) error {
			return c.JSON(200, map[string]any{"items": middlewares.RecentLogs(c.Request().Context())})
		})

		// Collections & Schema Portability.
		api.GET("/collections", colCtrl.List)
		api.POST("/collections", colCtrl.Create)
		api.GET("/collections/:name", colCtrl.Get)
		api.PATCH("/collections/:name", colCtrl.Update)
		api.DELETE("/collections/:name", colCtrl.Delete)
		api.GET("/schema/export", schemaCtrl.Export)
		api.POST("/schema/import", schemaCtrl.Import)

		// Dynamic Records & Search (with OptionalAuth for API Rules enforcement).
		recGroup := api.Group("/collections/:name", middlewares.OptionalAuth(authService))
		recGroup.GET("/records", recCtrl.List)
		recGroup.POST("/records", recCtrl.Create)
		recGroup.GET("/records/:id", recCtrl.Get)
		recGroup.PATCH("/records/:id", recCtrl.Update)
		recGroup.DELETE("/records/:id", recCtrl.Delete)
		recGroup.POST("/search/vector", recCtrl.VectorSearch)
	}
}
