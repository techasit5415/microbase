package controllers

import (
	"bytes"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/storage"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestThumbnailRejectsNegativeDimensions(t *testing.T) {
	for _, spec := range []string{"-1x10", "10x-1", "0x0", "2049x10", "bad"} {
		if _, _, ok := parseThumbSpec(spec); ok {
			t.Errorf("accepted %s", spec)
		}
	}
}
func TestThumbnailPreservesAtLeastOnePixel(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 100)))
	out, _, ok := resizeImage(&b, 0, 1)
	if !ok {
		t.Fatal("small thumbnail failed")
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil || img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 {
		t.Fatal("incorrect dimensions", err)
	}
}
func TestFileUploadDownload(t *testing.T) {
	e := echo.New()
	h := NewFileController(storage.NewSwitchableStorage(t.TempDir(), nil))
	e.POST("/files", h.Upload)
	e.GET("/files/:filename", h.Serve)
	var b bytes.Buffer
	form := multipart.NewWriter(&b)
	f, _ := form.CreateFormFile("file", "hello.txt")
	f.Write([]byte("hello"))
	form.Close()
	req := httptest.NewRequest("POST", "/files", &b)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/files/"+body["filename"].(string), nil))
	if rec.Code != 200 || rec.Body.String() != "hello" {
		t.Fatal(rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/files/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatal(rec.Code)
	}
}
