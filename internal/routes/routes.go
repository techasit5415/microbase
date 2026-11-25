package routes

import (
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
	"github.com/techasit5415/microbase/ui"
)

func Register(e *echo.Echo, pool *pgxpool.Pool, cacheStore cache.CacheStore, cfg *config.Config) {
	// Middlewares พื้นฐาน
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	ui.Register(e)
	// Controllers
	healthCtrl := controllers.NewHealthController(cacheStore)

	// Routes
	e.GET("/health", healthCtrl.Check)
	if pool != nil {
		hub := realtime.NewHub()
		colService := services.NewCollectionService(pool, cacheStore)
		recService := services.NewRecordService(pool, colService, hub)
		authService := services.NewAuthService(pool, cfg.JWTSecret)
		colCtrl := controllers.NewCollectionController(colService)
		schemaCtrl := controllers.NewSchemaController(colService)
		recCtrl := controllers.NewRecordController(recService)
		authCtrl := controllers.NewAuthController(authService)
		realtimeCtrl := controllers.NewRealtimeController(hub)

		api := e.Group("/api", middlewares.RateLimit(cacheStore, 100, time.Minute))

		// Realtime SSE
		api.GET("/realtime", realtimeCtrl.Stream)
		// Auth endpoints.
		api.POST("/auth/register", authCtrl.Register)
		api.POST("/auth/login", authCtrl.Login)
		api.GET("/auth/me", authCtrl.Me, middlewares.RequireAuth(authService))

		api.GET("/collections", colCtrl.List)
		api.POST("/collections", colCtrl.Create)
		api.GET("/collections/:name", colCtrl.Get)
		api.DELETE("/collections/:name", colCtrl.Delete)
		api.GET("/schema/export", schemaCtrl.Export)
		api.POST("/schema/import", schemaCtrl.Import)

		api.GET("/collections/:name/records", recCtrl.List)
		api.POST("/collections/:name/records", recCtrl.Create)
		api.GET("/collections/:name/records/:id", recCtrl.Get)
		api.PATCH("/collections/:name/records/:id", recCtrl.Update)
		api.DELETE("/collections/:name/records/:id", recCtrl.Delete)
		api.POST("/collections/:name/search/vector", recCtrl.VectorSearch)
		fileCtrl := controllers.NewFileController("./storage")
		api.POST("/files", fileCtrl.Upload)
		api.GET("/files/:filename", fileCtrl.Serve)
	}

}
