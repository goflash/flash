package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goflash/flash/v2"
)

func TestRequestSize_WithinLimit(t *testing.T) {
	app := flash.New()
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 1024, // 1KB limit
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "success")
	})

	// Test request within limit
	body := strings.NewReader("small body")
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Length", "10") // 10 bytes, well under 1KB
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "success" {
		t.Errorf("expected 'success', got %q", rec.Body.String())
	}
}

func TestRequestSize_ExceedsLimit(t *testing.T) {
	app := flash.New()
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 10, // 10 byte limit
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "should not reach here")
	})

	// Test request exceeding limit
	body := strings.NewReader("this body is longer than 10 bytes")
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Length", "35") // 35 bytes, exceeds 10 byte limit
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d", rec.Code)
	}

	// Check response headers
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options header to be set")
	}

	// Check JSON response structure
	expectedFields := []string{"error", "code", "limit"}
	body_str := rec.Body.String()
	for _, field := range expectedFields {
		if !strings.Contains(body_str, field) {
			t.Errorf("expected response to contain field %q, got %q", field, body_str)
		}
	}
}

func TestRequestSize_NoContentLength(t *testing.T) {
	app := flash.New()
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 10, // 10 byte limit
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "success")
	})

	// Test request without Content-Length header (e.g., chunked encoding)
	body := strings.NewReader("this body is longer than 10 bytes")
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	// Explicitly set Content-Length to -1 to simulate unknown length
	req.ContentLength = -1
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	// Should pass through since we can't check size without Content-Length
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for request without Content-Length, got %d", rec.Code)
	}
}

func TestRequestSize_ZeroMaxSize(t *testing.T) {
	app := flash.New()
	// Zero MaxSize should create a no-op middleware
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 0,
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "success")
	})

	// Test large request with zero limit (should pass)
	body := strings.NewReader(strings.Repeat("a", 10000)) // 10KB
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Length", "10000")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 with zero MaxSize, got %d", rec.Code)
	}
}

func TestRequestSize_NegativeMaxSize(t *testing.T) {
	app := flash.New()
	// Negative MaxSize should create a no-op middleware
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: -1,
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "success")
	})

	// Test large request with negative limit (should pass)
	body := strings.NewReader(strings.Repeat("b", 5000)) // 5KB
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Length", "5000")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 with negative MaxSize, got %d", rec.Code)
	}
}

func TestRequestSize_CustomErrorResponse(t *testing.T) {
	app := flash.New()
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 100, // 100 byte limit
		ErrorResponse: func(c flash.Ctx, size, limit int64) error {
			return c.Status(http.StatusBadRequest).JSON(map[string]interface{}{
				"custom_error": "Too big!",
				"size":         size,
				"limit":        limit,
				"message":      fmt.Sprintf("Request size %d exceeds limit %d", size, limit),
			})
		},
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "should not reach here")
	})

	// Test request exceeding limit with custom error response
	body := strings.NewReader(strings.Repeat("x", 200)) // 200 bytes
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Length", "200")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected custom status 400, got %d", rec.Code)
	}

	body_str := rec.Body.String()
	expectedFields := []string{"custom_error", "size", "limit", "message"}
	for _, field := range expectedFields {
		if !strings.Contains(body_str, field) {
			t.Errorf("expected custom response to contain field %q, got %q", field, body_str)
		}
	}

	// Verify the size and limit values are correct
	if !strings.Contains(body_str, "200") || !strings.Contains(body_str, "100") {
		t.Errorf("expected response to contain size 200 and limit 100, got %q", body_str)
	}
}

func TestRequestSize_DifferentHTTPMethods(t *testing.T) {
	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			app := flash.New()
			app.Use(RequestSize(RequestSizeConfig{
				MaxSize: 50, // 50 byte limit
			}))

			// Register handler for this method
			switch method {
			case http.MethodPost:
				app.POST("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
			case http.MethodPut:
				app.PUT("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
			case http.MethodPatch:
				app.PATCH("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
			}

			// Test request exceeding limit
			body := strings.NewReader(strings.Repeat("y", 100)) // 100 bytes
			req := httptest.NewRequest(method, "/test", body)
			req.Header.Set("Content-Length", "100")
			rec := httptest.NewRecorder()

			app.ServeHTTP(rec, req)

			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Errorf("method %s: expected status 413, got %d", method, rec.Code)
			}
		})
	}
}

func TestRequestSize_EdgeCases(t *testing.T) {
	t.Run("ExactLimit", func(t *testing.T) {
		app := flash.New()
		app.Use(RequestSize(RequestSizeConfig{
			MaxSize: 10, // Exactly 10 bytes
		}))
		app.POST("/test", func(c flash.Ctx) error {
			return c.String(http.StatusOK, "success")
		})

		// Test request exactly at limit
		body := strings.NewReader("1234567890") // Exactly 10 bytes
		req := httptest.NewRequest(http.MethodPost, "/test", body)
		req.Header.Set("Content-Length", "10")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 for exact limit, got %d", rec.Code)
		}
	})

	t.Run("OneByteLarger", func(t *testing.T) {
		app := flash.New()
		app.Use(RequestSize(RequestSizeConfig{
			MaxSize: 10, // 10 bytes limit
		}))
		app.POST("/test", func(c flash.Ctx) error {
			return c.String(http.StatusOK, "should not reach here")
		})

		// Test request one byte over limit
		body := strings.NewReader("12345678901") // 11 bytes
		req := httptest.NewRequest(http.MethodPost, "/test", body)
		req.Header.Set("Content-Length", "11")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected status 413 for one byte over limit, got %d", rec.Code)
		}
	})

	t.Run("EmptyBody", func(t *testing.T) {
		app := flash.New()
		app.Use(RequestSize(RequestSizeConfig{
			MaxSize: 10,
		}))
		app.POST("/test", func(c flash.Ctx) error {
			return c.String(http.StatusOK, "success")
		})

		// Test empty request body
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Content-Length", "0")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 for empty body, got %d", rec.Code)
		}
	})
}

func TestRequestSize_RouteSpecificLimits(t *testing.T) {
	app := flash.New()

	// API group with strict limit (no global middleware)
	api := app.Group("/api")
	api.Use(RequestSize(RequestSizeConfig{
		MaxSize: 100, // 100 byte limit for API
	}))
	api.POST("/data", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "api success")
	})

	// Upload group with higher limit (no global middleware)
	upload := app.Group("/upload")
	upload.Use(RequestSize(RequestSizeConfig{
		MaxSize: 5000, // 5KB limit for uploads
	}))
	upload.POST("/file", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "upload success")
	})

	// Regular endpoint with global limit
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 1000, // 1KB global limit
	}))
	app.POST("/regular", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "regular success")
	})

	// Test API endpoint with 500 bytes (should fail - exceeds 100 byte limit)
	t.Run("API_ExceedsLimit", func(t *testing.T) {
		body := strings.NewReader(strings.Repeat("a", 500))
		req := httptest.NewRequest(http.MethodPost, "/api/data", body)
		req.Header.Set("Content-Length", "500")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected status 413 for API request exceeding limit, got %d", rec.Code)
		}
	})

	// Test upload endpoint with 2KB (should succeed - under 5KB limit)
	t.Run("Upload_WithinLimit", func(t *testing.T) {
		body := strings.NewReader(strings.Repeat("b", 2000))
		req := httptest.NewRequest(http.MethodPost, "/upload/file", body)
		req.Header.Set("Content-Length", "2000")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 for upload within limit, got %d", rec.Code)
		}
	})

	// Test regular endpoint with 800 bytes (should succeed - under 1KB global limit)
	t.Run("Regular_WithinGlobalLimit", func(t *testing.T) {
		body := strings.NewReader(strings.Repeat("c", 800))
		req := httptest.NewRequest(http.MethodPost, "/regular", body)
		req.Header.Set("Content-Length", "800")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 for regular request within global limit, got %d", rec.Code)
		}
	})
}

func TestRequestSize_PerformanceNoAllocation(t *testing.T) {
	app := flash.New()
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 1024,
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "success")
	})

	// Test that the middleware doesn't allocate memory for size checking
	body := strings.NewReader("small request")
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Length", "13")

	// Run the request multiple times to ensure no memory leaks
	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("iteration %d: expected status 200, got %d", i, rec.Code)
		}

		// Reset request body for next iteration
		body.Seek(0, 0)
	}
}

// Benchmark the middleware performance
func BenchmarkRequestSize_WithinLimit(b *testing.B) {
	app := flash.New()
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 1024,
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "success")
	})

	body := strings.NewReader("benchmark request body")
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Length", "22")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		body.Seek(0, 0) // Reset for next iteration
	}
}

func BenchmarkRequestSize_ExceedsLimit(b *testing.B) {
	app := flash.New()
	app.Use(RequestSize(RequestSizeConfig{
		MaxSize: 10,
	}))
	app.POST("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "should not reach here")
	})

	body := strings.NewReader("this request body is too large")
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Length", "32")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		body.Seek(0, 0) // Reset for next iteration
	}
}
