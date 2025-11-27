package middleware

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goflash/flash/v2"
	"github.com/stretchr/testify/assert"
)

func TestDecompress_Gzip(t *testing.T) {
	app := flash.New()
	app.Use(Decompress())
	app.POST("/", func(c flash.Ctx) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	})

	// Compress test data
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("hello world"))
	_ = gz.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello world", rec.Body.String())
}

func TestDecompress_Deflate(t *testing.T) {
	app := flash.New()
	app.Use(Decompress())
	app.POST("/", func(c flash.Ctx) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	})

	// Compress test data with deflate
	var buf bytes.Buffer
	fl, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	_, _ = fl.Write([]byte("hello world"))
	_ = fl.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Encoding", "deflate")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello world", rec.Body.String())
}

func TestDecompress_NoEncoding(t *testing.T) {
	app := flash.New()
	app.Use(Decompress())
	app.POST("/", func(c flash.Ctx) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("hello world"))
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello world", rec.Body.String())
}

func TestDecompress_InvalidGzip(t *testing.T) {
	app := flash.New()
	app.Use(Decompress())
	app.POST("/", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("invalid gzip data"))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDecompress_EmptyBody(t *testing.T) {
	app := flash.New()
	app.Use(Decompress())
	app.POST("/", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDecompress_CustomEncodings(t *testing.T) {
	app := flash.New()
	app.Use(Decompress(DecompressConfig{
		AllowedEncodings: []string{"gzip"},
	}))
	app.POST("/", func(c flash.Ctx) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	})

	// Test gzip (allowed)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("hello"))
	_ = gz.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())

	// Test deflate (not allowed, should pass through)
	buf2 := bytes.NewBufferString("hello")
	req2 := httptest.NewRequest(http.MethodPost, "/", buf2)
	req2.Header.Set("Content-Encoding", "deflate")
	rec2 := httptest.NewRecorder()

	app.ServeHTTP(rec2, req2)
	// Should fail because deflate is not decompressed
	body, _ := io.ReadAll(rec2.Body)
	assert.NotEqual(t, "hello", string(body))
}

func TestDecompress_BindJSON(t *testing.T) {
	app := flash.New()
	app.Use(Decompress())
	app.POST("/", func(c flash.Ctx) error {
		var data struct {
			Name string `json:"name"`
		}
		if err := c.BindJSON(&data); err != nil {
			return err
		}
		return c.String(http.StatusOK, data.Name)
	})

	// Compress JSON
	jsonData := `{"name":"test"}`
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(jsonData))
	_ = gz.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test", rec.Body.String())
}
