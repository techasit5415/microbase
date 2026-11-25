package ui

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"
)

//go:embed all:build
var buildFS embed.FS

// Register mounts the embedded SvelteKit SPA under /_/.
func Register(e *echo.Echo) {
	sub, _ := fs.Sub(buildFS, "build")
	fileServer := http.StripPrefix("/_/", http.FileServer(http.FS(sub)))

	e.GET("/_", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/_/")
	})
	e.GET("/_/", echo.WrapHandler(fileServer))
	e.GET("/_/*", echo.WrapHandler(fileServer))
}
