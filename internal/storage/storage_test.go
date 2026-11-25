package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLocalStorageLifecycle(t *testing.T) {
	s := NewLocalStorage(t.TempDir())
	c := context.Background()
	if err := s.Save(c, "../hello.txt", "text/plain", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	f, ct, err := s.Open(c, "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(f)
	f.Close()
	if string(data) != "hello" || !strings.HasPrefix(ct, "text/plain") {
		t.Fatal(string(data), ct)
	}
	if err = s.Delete(c, "hello.txt"); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(c, "hello.txt"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Open(c, "hello.txt"); !errors.Is(err, ErrFileNotFound) {
		t.Fatal(err)
	}
}
func TestS3ObjectKeysAreNotDoubleEscaped(t *testing.T) {
	for _, pathStyle := range []bool{true, false} {
		s, err := NewS3Storage(S3Config{Endpoint: "https://example.test", Bucket: "bucket", AccessKey: "key", SecretKey: "secret", ForcePathStyle: pathStyle})
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"folder/a b.txt", "ไทย.txt", "a+b%20.txt"} {
			raw, _, err := s.objectURL(key)
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(raw)
			want := "/" + key
			if pathStyle {
				want = "/bucket" + want
			}
			if u.Path != want {
				t.Errorf("key %q got path %q want %q", key, u.Path, want)
			}
		}
	}
}
func TestS3HTTPRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("missing signature")
		}
		if r.URL.Path != "/bucket/file.txt" && r.URL.Path != "/bucket" {
			t.Error(r.URL.Path)
		}
		switch r.Method {
		case "GET":
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "payload")
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			if string(b) != "payload" {
				t.Error(string(b))
			}
		case "HEAD", "DELETE":
		default:
			t.Error(r.Method)
		}
	}))
	defer server.Close()
	s, _ := NewS3Storage(S3Config{Endpoint: server.URL, Bucket: "bucket", AccessKey: "test", SecretKey: "test", ForcePathStyle: true})
	c := context.Background()
	if err := s.TestConnection(c); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(c, "file.txt", "text/plain", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	f, ct, err := s.Open(c, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "payload" || ct != "text/plain" {
		t.Fatal(string(b), ct)
	}
	if err := s.Delete(c, "file.txt"); err != nil {
		t.Fatal(err)
	}
}
