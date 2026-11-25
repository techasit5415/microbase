package routes

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/controllers"
	"github.com/techasit5415/microbase/internal/services"
)

func Register(e *echo.Echo, pool *pgxpool.Pool, cacheStore cache.CacheStore) {
	// Middlewares พื้นฐาน
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Controllers
	healthCtrl := controllers.NewHealthController(cacheStore)

	// Routes
	e.GET("/health", healthCtrl.Check)
	if pool != nil {
		colService := services.NewCollectionService(pool, cacheStore)
		colCtrl := controllers.NewCollectionController(colService)
		schemaCtrl := controllers.NewSchemaController(colService)
		api := e.Group("/api")
		api.GET("/collections", colCtrl.List)
		api.POST("/collections", colCtrl.Create)
		api.GET("/collections/:name", colCtrl.Get)
		api.DELETE("/collections/:name", colCtrl.Delete)
		api.GET("/schema/export", schemaCtrl.Export)
		api.POST("/schema/import", schemaCtrl.Import)
	}
}
