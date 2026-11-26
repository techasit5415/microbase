package controllers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/storage"
)

const maxUploadBytes = 20 << 20 // 20 MiB hard ceiling

// FileController handles file uploads, downloads, short-lived file tokens, and PocketBase-style ?thumb= resizing.
type FileController struct {
	storage *storage.SwitchableStorage
	auth    *services.AuthService
}

func NewFileController(store *storage.SwitchableStorage, auth ...*services.AuthService) *FileController {
	var authSvc *services.AuthService
	if len(auth) > 0 {
		authSvc = auth[0]
	}
	return &FileController{storage: store, auth: authSvc}
}

// CreateToken handles POST /api/files/token — mints a 2-minute signed token for protected file downloads.
func (h *FileController) CreateToken(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil || h.auth == nil {
		return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required to generate a file token")
	}
	tok, err := h.auth.GenerateFileToken(claims)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "TOKEN_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"token": tok})
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

// Serve handles GET /api/files/:filename with optional ?token=... validation and ?thumb=WxH[f|t|b] on-the-fly resizing + caching.
func (h *FileController) Serve(c echo.Context) error {
	safeName := filepath.Base(c.Param("filename"))
	if safeName == "." || safeName == "/" || safeName == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_FILENAME", "invalid filename")
	}

	// If a short-lived file ?token=... is provided, validate it strictly.
	if fileTok := strings.TrimSpace(c.QueryParam("token")); fileTok != "" && h.auth != nil {
		if _, err := h.auth.ValidateFileToken(fileTok); err != nil {
			return response.Error(c, http.StatusForbidden, "INVALID_FILE_TOKEN", "invalid or expired file token")
		}
	}

	thumbSpec := strings.TrimSpace(c.QueryParam("thumb"))
	if thumbSpec != "" {
		if w, hDim, mode, ok := parseThumbSpec(thumbSpec); ok {
			cacheKey := fmt.Sprintf("thumb_%dx%d%s_%s", w, hDim, mode, safeName)
			if cachedRC, cachedCT, err := h.storage.Open(c.Request().Context(), cacheKey); err == nil {
				defer cachedRC.Close()
				c.Response().Header().Set("Cache-Control", "public, max-age=2592000")
				return c.Stream(http.StatusOK, cachedCT, cachedRC)
			}

			rc, contentType, err := h.storage.Open(c.Request().Context(), safeName)
			if errors.Is(err, storage.ErrFileNotFound) {
				return response.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
			}
			if err != nil {
				return response.Error(c, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
			}
			defer rc.Close()

			if strings.HasPrefix(contentType, "image/") {
				if resized, outCT, ok := resizeImageMode(rc, w, hDim, mode); ok {
					_ = h.storage.Save(c.Request().Context(), cacheKey, outCT, resized)
					c.Response().Header().Set("Cache-Control", "public, max-age=2592000")
					return c.Blob(http.StatusOK, outCT, resized)
				}
			}
		}
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

// parseThumbSpec parses PocketBase-style thumb specs: WxH (center crop), WxHf (fit), WxHt (top crop), WxHb (bottom crop), 0xH, Wx0.
func parseThumbSpec(spec string) (int, int, string, bool) {
	s := strings.ToLower(strings.TrimSpace(spec))
	mode := ""
	if strings.HasSuffix(s, "f") || strings.HasSuffix(s, "t") || strings.HasSuffix(s, "b") {
		mode = s[len(s)-1:]
		s = s[:len(s)-1]
	}
	parts := strings.Split(s, "x")
	if len(parts) != 2 {
		return 0, 0, "", false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w < 0 || h < 0 || (w == 0 && h == 0) || w > 2048 || h > 2048 {
		return 0, 0, "", false
	}
	return w, h, mode, true
}

func resizeImage(r io.Reader, targetW, targetH int) ([]byte, string, bool) {
	return resizeImageMode(r, targetW, targetH, "")
}

func resizeImageMode(r io.Reader, targetW, targetH int, mode string) ([]byte, string, bool) {
	src, format, err := image.Decode(r)
	if err != nil {
		return nil, "", false
	}
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, "", false
	}

	cropX, cropY, cropW, cropH := sb.Min.X, sb.Min.Y, srcW, srcH
	if targetW == 0 {
		targetW = max(1, srcW*targetH/srcH)
	} else if targetH == 0 {
		targetH = max(1, srcH*targetW/srcW)
	} else if mode == "f" {
		// Fit inside WxH preserving aspect ratio without cropping
		scaleW := float64(targetW) / float64(srcW)
		scaleH := float64(targetH) / float64(srcH)
		scale := scaleW
		if scaleH < scale {
			scale = scaleH
		}
		targetW = max(1, int(float64(srcW)*scale+0.5))
		targetH = max(1, int(float64(srcH)*scale+0.5))
	} else {
		// Crop to match target aspect ratio (center default, or top 't', or bottom 'b')
		srcRatio := float64(srcW) / float64(srcH)
		dstRatio := float64(targetW) / float64(targetH)
		if srcRatio > dstRatio {
			cropW = max(1, int(float64(srcH)*dstRatio+0.5))
			cropX = sb.Min.X + (srcW-cropW)/2
		} else if srcRatio < dstRatio {
			cropH = max(1, int(float64(srcW)/dstRatio+0.5))
			switch mode {
			case "t":
				cropY = sb.Min.Y
			case "b":
				cropY = sb.Min.Y + (srcH - cropH)
			default:
				cropY = sb.Min.Y + (srcH-cropH)/2
			}
		}
	}

	if targetW < 1 || targetH < 1 || targetW > 2048 || targetH > 2048 {
		return nil, "", false
	}

	// Bilinear resampling for crisp thumbnails
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xRatio := float64(cropW) / float64(targetW)
	yRatio := float64(cropH) / float64(targetH)
	for y := 0; y < targetH; y++ {
		sy := float64(cropY) + (float64(y)+0.5)*yRatio - 0.5
		y0 := int(sy)
		if y0 < sb.Min.Y {
			y0 = sb.Min.Y
		}
		y1 := min(y0+1, sb.Max.Y-1)
		fy := sy - float64(y0)
		if fy < 0 {
			fy = 0
		}
		for x := 0; x < targetW; x++ {
			sx := float64(cropX) + (float64(x)+0.5)*xRatio - 0.5
			x0 := int(sx)
			if x0 < sb.Min.X {
				x0 = sb.Min.X
			}
			x1 := min(x0+1, sb.Max.X-1)
			fx := sx - float64(x0)
			if fx < 0 {
				fx = 0
			}
			c00 := color.NRGBAModel.Convert(src.At(x0, y0)).(color.NRGBA)
			c10 := color.NRGBAModel.Convert(src.At(x1, y0)).(color.NRGBA)
			c01 := color.NRGBAModel.Convert(src.At(x0, y1)).(color.NRGBA)
			c11 := color.NRGBAModel.Convert(src.At(x1, y1)).(color.NRGBA)
			dst.Set(x, y, color.NRGBA{
				R: bilerp(c00.R, c10.R, c01.R, c11.R, fx, fy),
				G: bilerp(c00.G, c10.G, c01.G, c11.G, fx, fy),
				B: bilerp(c00.B, c10.B, c01.B, c11.B, fx, fy),
				A: bilerp(c00.A, c10.A, c01.A, c11.A, fx, fy),
			})
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

func bilerp(c00, c10, c01, c11 uint8, fx, fy float64) uint8 {
	top := float64(c00)*(1-fx) + float64(c10)*fx
	bot := float64(c01)*(1-fx) + float64(c11)*fx
	v := top*(1-fy) + bot*fy
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
