package main

import (
	"bytes"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadEmbeddedAssets_LoadsTemplateStaticAndSpec(t *testing.T) {
	tmpl, staticFS, spec, err := loadEmbeddedAssets()
	if err != nil {
		t.Fatalf("loadEmbeddedAssets error = %v", err)
	}
	if tmpl == nil {
		t.Fatal("expected non-nil template")
	}
	if staticFS == nil {
		t.Fatal("expected non-nil static filesystem")
	}
	if len(spec) == 0 {
		t.Fatal("expected non-empty OpenAPI spec")
	}
}

func TestNewApp_BuildsAppFromConfig(t *testing.T) {
	cfg := runConfig{
		simBaseURL:     "http://sim:8000",
		swaggerEnabled: true,
		swaggerToken:   "tok",
		simCapTTL:      7 * time.Second,
		upstream:       upstreamConfig{Timeout: 3 * time.Second, MaxBodyBytes: 1024},
	}
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	mc := newMetricsCollector()

	a := newApp(logger, mc, nil, []byte("spec"), cfg)

	if a.simBaseURL != cfg.simBaseURL {
		t.Errorf("simBaseURL = %q, want %q", a.simBaseURL, cfg.simBaseURL)
	}
	if !a.swaggerEnabled || a.swaggerToken != "tok" {
		t.Errorf("swagger fields not propagated: enabled=%v token=%q", a.swaggerEnabled, a.swaggerToken)
	}
	if a.simCapTTL != 7*time.Second {
		t.Errorf("simCapTTL = %v, want 7s", a.simCapTTL)
	}
	if a.maxBodyBytes != 1024 {
		t.Errorf("maxBodyBytes = %d, want 1024", a.maxBodyBytes)
	}
	if a.httpClient == nil || a.httpClient.Timeout != 3*time.Second {
		t.Errorf("httpClient not configured with upstream timeout: %+v", a.httpClient)
	}
}

func buildTestHandler(t *testing.T, cfg runConfig) http.Handler {
	t.Helper()
	tmpl, staticFS, spec, err := loadEmbeddedAssets()
	if err != nil {
		t.Fatalf("loadEmbeddedAssets: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	mc := newMetricsCollector()
	a := newApp(logger, mc, tmpl, spec, cfg)
	return newHTTPHandler(logger, mc, a, staticFS, cfg)
}

func TestNewHTTPHandler_MetricsRouteGatedByConfig(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		h := buildTestHandler(t, runConfig{metricsEnabled: true, simBaseURL: "http://sim:8000"})
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 when metrics enabled", rr.Code)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		// No dedicated route is registered when disabled, so the request
		// falls through to the "GET /" catch-all (handleIndex) — same as
		// hitting "/" directly. There is no dedicated 404 for this case.
		h := buildTestHandler(t, runConfig{metricsEnabled: false, simBaseURL: "http://sim:8000"})
		assertFallsThroughToIndex(t, h, "/metrics")
	})
}

func TestNewHTTPHandler_PprofRoutesGatedByConfig(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		h := buildTestHandler(t, runConfig{pprofEnabled: true, simBaseURL: "http://sim:8000"})
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
		if rr.Code == http.StatusNotFound {
			t.Fatal("expected pprof route to be registered")
		}
	})
	t.Run("disabled", func(t *testing.T) {
		h := buildTestHandler(t, runConfig{pprofEnabled: false, simBaseURL: "http://sim:8000"})
		assertFallsThroughToIndex(t, h, "/debug/pprof/")
	})
}

func TestNewHTTPHandler_SwaggerRoutesGatedByConfig(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		h := buildTestHandler(t, runConfig{swaggerEnabled: true, simBaseURL: "http://sim:8000"})
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/swagger", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 when swagger enabled", rr.Code)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		h := buildTestHandler(t, runConfig{swaggerEnabled: false, simBaseURL: "http://sim:8000"})
		assertFallsThroughToIndex(t, h, "/swagger")
	})
}

// assertFallsThroughToIndex verifies that path, when no dedicated route is
// registered for it, is served by the "GET /" catch-all (handleIndex) —
// i.e. it renders identically to a direct request for "/". The dashboard's
// root route is a subtree pattern ("GET /"), so any unmatched path falls
// through to it rather than 404ing; there is no separate not-found case to
// assert here.
func assertFallsThroughToIndex(t *testing.T, h http.Handler, path string) {
	t.Helper()

	rootRR := httptest.NewRecorder()
	h.ServeHTTP(rootRR, httptest.NewRequest(http.MethodGet, "/", nil))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (falls through to index)", rr.Code)
	}
	if rr.Body.String() != rootRR.Body.String() {
		t.Fatalf("expected %q to render the same catch-all index page as \"/\"", path)
	}
}

func TestLogRunConfig_LogsKeyFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	a := &app{swaggerEnabled: true}
	cfg := runConfig{
		http:      serverConfig{ListenAddr: ":8080"},
		simCapTTL: 5 * time.Second,
	}

	logRunConfig(logger, a, cfg)

	out := buf.String()
	for _, want := range []string{`"msg":"starting dashboard server"`, `"listen_addr":":8080"`, `"swagger_enabled":true`} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q, got: %s", want, out)
		}
	}
}

func TestServeWithShutdown_ListenAddrInUse_ReturnsWrappedError(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer occupied.Close()

	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	server := &http.Server{Addr: occupied.Addr().String()}

	err = serveWithShutdown(logger, server, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when the listen address is already in use")
	}
}
