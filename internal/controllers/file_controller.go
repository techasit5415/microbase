package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/storage"
)

const maxUploadBytes = 20 << 20 // 20 MiB hard ceiling

// FileController handles file uploads and downloads via Local or S3 storage.
type FileController struct {
	storage *storage.SwitchableStorage
}

// NewFileController constructs a FileController backed by SwitchableStorage.
func NewFileController(store *storage.SwitchableStorage) *FileController {
	return &FileController{storage: store}
}

// Upload handles POST /api/files (multipart/form-data with field "file").
func (h *FileController) Upload(c echo.Context) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "MISSING_FILE", "form field 'file' is required")
	}
	if fileHeader.Size > maxUploadBytes {
		return response.Error(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file exceeds 20MB limit")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, maxUploadBytes+1))
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	if int64(len(data)) > maxUploadBytes {
		return response.Error(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file exceeds 20MB limit")
	}

	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	prefix := randomHex(8)
	safeName := filepath.Base(fileHeader.Filename)
	storedName := fmt.Sprintf("%s_%s", prefix, safeName)

	if err := h.storage.Save(c.Request().Context(), storedName, contentType, data); err != nil {
		return response.Error(c, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
	}

	return response.Created(c, map[string]any{
		"filename":     storedName,
		"url":          "/api/files/" + storedName,
		"size":         len(data),
		"content_type": contentType,
		"backend":      h.storage.Backend(),
	})
}

// Serve handles GET /api/files/:filename.
func (h *FileController) Serve(c echo.Context) error {
	safeName := filepath.Base(c.Param("filename"))
	if safeName == "." || safeName == "/" || safeName == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_FILENAME", "invalid filename")
	}

	rc, contentType, err := h.storage.Open(c.Request().Context(), safeName)
	if errors.Is(err, storage.ErrFileNotFound) {
		return response.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
	}
	defer rc.Close()

	return c.Stream(http.StatusOK, contentType, rc)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
