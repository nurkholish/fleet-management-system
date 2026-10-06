package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func TestRequestLogger_GeneratesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger(zerolog.Nop()))
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	reqID := w.Header().Get("X-Request-ID")
	if reqID == "" {
		t.Fatal("expected X-Request-ID header")
	}
}

func TestRequestLogger_PropagatesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger(zerolog.Nop()))
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "custom-id-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "custom-id-123" {
		t.Errorf("expected propagated id, got %q", got)
	}
}

func TestRequestLogger_LogsWithRealLogger(t *testing.T) {
	// Use a real logger writing into a buffer to exercise the log path.
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	baseLog := zerolog.New(&buf)

	r := gin.New()
	r.Use(RequestLogger(baseLog))
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.0.2.10:12345" // exercise ClientIP path
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if buf.Len() == 0 {
		t.Fatal("expected logger to write at least one line")
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("http_request")) {
		t.Errorf("log missing 'http_request' msg: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("192.0.2.10")) {
		t.Errorf("log missing client ip: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("\"status\":200")) {
		t.Errorf("log missing status:200: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("request_id")) {
		t.Errorf("log missing request_id: %s", out)
	}
}

func TestRequestLogger_LogsErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	baseLog := zerolog.New(&buf)

	r := gin.New()
	r.Use(RequestLogger(baseLog))
	r.GET("/boom", func(c *gin.Context) {
		c.String(http.StatusInternalServerError, "boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("\"status\":500")) {
		t.Errorf("log missing status:500: %s", out)
	}
}

// ============================================================
// LoggerFrom
// ============================================================

func TestLoggerFrom_ReturnsInjectedLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger(zerolog.Nop()))

	var captured zerolog.Logger
	r.GET("/x", func(c *gin.Context) {
		captured = LoggerFrom(c, zerolog.Nop())
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// just verify no panic and we got a logger
	_ = captured
}

func TestLoggerFrom_FallbackWhenNotInjected(t *testing.T) {
	// No RequestLogger middleware — LoggerFrom should return the base logger.
	gin.SetMode(gin.TestMode)
	r := gin.New()

	var captured zerolog.Logger
	r.GET("/x", func(c *gin.Context) {
		captured = LoggerFrom(c, zerolog.Nop())
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Fallback path — no panic. captured should equal zerolog.Nop() level.
	_ = captured
}

func TestLoggerFrom_WrongTypeInContext(t *testing.T) {
	// Inject a wrong type into ctx — LoggerFrom should fall back to base logger.
	gin.SetMode(gin.TestMode)
	r := gin.New()

	var captured zerolog.Logger
	r.GET("/x", func(c *gin.Context) {
		c.Set(ctxLoggerKey, "not-a-logger") // wrong type
		captured = LoggerFrom(c, zerolog.Nop())
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	_ = captured // returned fallback, no panic
}
