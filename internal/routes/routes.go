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
		recService := services.NewRecordService(pool, colService)

		colCtrl := controllers.NewCollectionController(colService)
		schemaCtrl := controllers.NewSchemaController(colService)
		recCtrl := controllers.NewRecordController(recService)

		api := e.Group("/api")
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
	}
}
