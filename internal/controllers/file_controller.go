package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/response"
)

const maxUploadBytes = 10 << 20 // 10 MiB

// FileController handles local file uploads and downloads.
type FileController struct {
	storageDir string
}

// NewFileController constructs a FileController and ensures storageDir exists.
func NewFileController(storageDir string) *FileController {
	_ = os.MkdirAll(storageDir, 0o755)
	return &FileController{storageDir: storageDir}
}

// Upload handles POST /api/files (multipart/form-data with field "file").
func (h *FileController) Upload(c echo.Context) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "MISSING_FILE", "form field 'file' is required")
	}
	if fileHeader.Size > maxUploadBytes {
		return response.Error(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file exceeds 10MB limit")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	defer src.Close()

	// Generate a random prefix and strip directory components to prevent path traversal.
	prefix := randomHex(8)
	safeName := filepath.Base(fileHeader.Filename)
	storedName := fmt.Sprintf("%s_%s", prefix, safeName)
	dstPath := filepath.Join(h.storageDir, storedName)

	dst, err := os.Create(dstPath)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	defer dst.Close()

	written, err := io.Copy(dst, io.LimitReader(src, maxUploadBytes+1))
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return response.Created(c, map[string]any{
		"filename": storedName,
		"url":      "/api/files/" + storedName,
		"size":     written,
	})
}

// Serve handles GET /api/files/:filename.
func (h *FileController) Serve(c echo.Context) error {
	safeName := filepath.Base(c.Param("filename"))
	if safeName == "." || safeName == "/" || safeName == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_FILENAME", "invalid filename")
	}

	fullPath := filepath.Join(h.storageDir, safeName)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return response.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
	}
	return c.File(fullPath)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
