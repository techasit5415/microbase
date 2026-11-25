package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/controllers"
)

func Register(e *echo.Echo, cacheStore cache.CacheStore) {
	// Middlewares พื้นฐาน
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Controllers
	healthCtrl := controllers.NewHealthController(cacheStore)

	// Routes
	e.GET("/health", healthCtrl.Check)
}
