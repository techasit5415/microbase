package controllers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/storage"
)

const maxUploadBytes = 20 << 20 // 20 MiB hard ceiling

// FileController handles file uploads, downloads, and on-the-fly ?thumb=WxH image resizing.
type FileController struct {
	storage *storage.SwitchableStorage
}

func NewFileController(store *storage.SwitchableStorage) *FileController {
	return &FileController{storage: store}
}

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

// Serve handles GET /api/files/:filename with optional ?thumb=WxH on-the-fly resizing.
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

	thumbSpec := strings.TrimSpace(c.QueryParam("thumb"))
	if thumbSpec != "" && (strings.HasPrefix(contentType, "image/jpeg") || strings.HasPrefix(contentType, "image/png")) {
		if w, hDim, ok := parseThumbSpec(thumbSpec); ok {
			if resized, outCT, ok := resizeImage(rc, w, hDim); ok {
				return c.Blob(http.StatusOK, outCT, resized)
			}
			// Re-open original if decode failed
			rc2, ct2, err2 := h.storage.Open(c.Request().Context(), safeName)
			if err2 == nil {
				defer rc2.Close()
				return c.Stream(http.StatusOK, ct2, rc2)
			}
		}
	}

	return c.Stream(http.StatusOK, contentType, rc)
}

func parseThumbSpec(spec string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(spec), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || (w <= 0 && h <= 0) || w > 2048 || h > 2048 {
		return 0, 0, false
	}
	return w, h, true
}

func resizeImage(r io.Reader, targetW, targetH int) ([]byte, string, bool) {
	src, format, err := image.Decode(r)
	if err != nil {
		return nil, "", false
	}
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, "", false
	}
	if targetW == 0 {
		targetW = srcW * targetH / srcH
	}
	if targetH == 0 {
		targetH = srcH * targetW / srcW
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	for y := 0; y < targetH; y++ {
		sy := sb.Min.Y + y*srcH/targetH
		for x := 0; x < targetW; x++ {
			sx := sb.Min.X + x*srcW/targetW
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	var buf bytes.Buffer
	if format == "png" {
		if err := png.Encode(&buf, dst); err != nil {
			return nil, "", false
		}
		return buf.Bytes(), "image/png", true
	}
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", false
	}
	return buf.Bytes(), "image/jpeg", true
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
