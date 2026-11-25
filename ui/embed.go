package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

//go:embed all:build
var buildFS embed.FS

// Register mounts the embedded SvelteKit SPA under /_/ with SPA fallback routing.
func Register(e *echo.Echo) {
	sub, _ := fs.Sub(buildFS, "build")
	indexHTML, _ := fs.ReadFile(sub, "index.html")
	fileServer := http.StripPrefix("/_/", http.FileServer(http.FS(sub)))

	handler := func(c echo.Context) error {
		rel := strings.TrimPrefix(c.Request().URL.Path, "/_/")
		if rel != "" {
			if info, err := fs.Stat(sub, rel); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(c.Response(), c.Request())
				return nil
			}
		}
		return c.HTMLBlob(http.StatusOK, indexHTML)
	}

	e.GET("/_", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/_/collections")
	})
	e.GET("/_/", handler)
	e.GET("/_/*", handler)
}
