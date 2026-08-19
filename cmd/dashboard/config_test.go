package main

import (
	"strings"
	"testing"
	"time"
)

func TestGetEnv_ReturnsValueWhenSet(t *testing.T) {
	t.Setenv("DASH_TEST_STR", "  configured  ")
	if got := getEnv("DASH_TEST_STR", "fallback"); got != "configured" {
		t.Fatalf("getEnv = %q, want trimmed %q", got, "configured")
	}
}

func TestGetEnv_ReturnsFallbackWhenUnset(t *testing.T) {
	t.Setenv("DASH_TEST_STR", "")
	if got := getEnv("DASH_TEST_STR", "fallback"); got != "fallback" {
		t.Fatalf("getEnv = %q, want fallback", got)
	}
}

func TestGetEnvInt_ParsesValidInt(t *testing.T) {
	t.Setenv("DASH_TEST_INT", "42")
	if got := getEnvInt("DASH_TEST_INT", 7); got != 42 {
		t.Fatalf("getEnvInt = %d, want 42", got)
	}
}

func TestGetEnvInt_ReturnsFallbackWhenUnset(t *testing.T) {
	t.Setenv("DASH_TEST_INT", "")
	if got := getEnvInt("DASH_TEST_INT", 7); got != 7 {
		t.Fatalf("getEnvInt = %d, want fallback 7", got)
	}
}

func TestGetEnvInt_ReturnsFallbackWhenInvalid(t *testing.T) {
	t.Setenv("DASH_TEST_INT", "not-a-number")
	if got := getEnvInt("DASH_TEST_INT", 7); got != 7 {
		t.Fatalf("getEnvInt = %d, want fallback 7 on parse error", got)
	}
}

func TestGetEnvBool_RecognizesTruthyAndFalsyStrings(t *testing.T) {
	cases := []struct {
		raw      string
		fallback bool
		want     bool
	}{
		{"1", false, true},
		{"true", false, true},
		{"T", false, true},
		{"yes", false, true},
		{"y", false, true},
		{"ON", false, true},
		{"0", true, false},
		{"false", true, false},
		{"F", true, false},
		{"no", true, false},
		{"n", true, false},
		{"OFF", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Setenv("DASH_TEST_BOOL", tc.raw)
			if got := getEnvBool("DASH_TEST_BOOL", tc.fallback); got != tc.want {
				t.Fatalf("getEnvBool(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestGetEnvBool_ReturnsFallbackWhenUnsetOrUnrecognized(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		t.Setenv("DASH_TEST_BOOL", "")
		if got := getEnvBool("DASH_TEST_BOOL", true); got != true {
			t.Fatalf("getEnvBool = %v, want fallback true", got)
		}
	})
	t.Run("unrecognized", func(t *testing.T) {
		t.Setenv("DASH_TEST_BOOL", "maybe")
		if got := getEnvBool("DASH_TEST_BOOL", true); got != true {
			t.Fatalf("getEnvBool = %v, want fallback true for unrecognized value", got)
		}
	})
}

func TestGetEnvDuration_ParsesValidDuration(t *testing.T) {
	t.Setenv("DASH_TEST_DUR", "250ms")
	if got := getEnvDuration("DASH_TEST_DUR", time.Second); got != 250*time.Millisecond {
		t.Fatalf("getEnvDuration = %v, want 250ms", got)
	}
}

func TestGetEnvDuration_ReturnsFallbackWhenUnset(t *testing.T) {
	t.Setenv("DASH_TEST_DUR", "")
	if got := getEnvDuration("DASH_TEST_DUR", time.Second); got != time.Second {
		t.Fatalf("getEnvDuration = %v, want fallback 1s", got)
	}
}

func TestGetEnvDuration_ReturnsFallbackWhenInvalidOrNegative(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		t.Setenv("DASH_TEST_DUR", "not-a-duration")
		if got := getEnvDuration("DASH_TEST_DUR", time.Second); got != time.Second {
			t.Fatalf("getEnvDuration = %v, want fallback 1s", got)
		}
	})
	t.Run("negative", func(t *testing.T) {
		t.Setenv("DASH_TEST_DUR", "-5s")
		if got := getEnvDuration("DASH_TEST_DUR", time.Second); got != time.Second {
			t.Fatalf("getEnvDuration = %v, want fallback 1s for negative duration", got)
		}
	})
}

func TestNormalizedSimCapTTL_ReturnsFallbackWhenNonPositive(t *testing.T) {
	for _, ttl := range []time.Duration{0, -1 * time.Second} {
		if got := normalizedSimCapTTL(ttl); got != 5*time.Second {
			t.Fatalf("normalizedSimCapTTL(%v) = %v, want 5s fallback", ttl, got)
		}
	}
}

func TestNormalizedSimCapTTL_ReturnsInputWhenPositive(t *testing.T) {
	if got := normalizedSimCapTTL(30 * time.Second); got != 30*time.Second {
		t.Fatalf("normalizedSimCapTTL = %v, want unchanged 30s", got)
	}
}

func clearRunConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SIMULATOR_BASE_URL", "DASHBOARD_POLL_MS", "SWAGGER_ENABLED", "SWAGGER_TOKEN",
		"DASHBOARD_PORT", "METRICS_ENABLED", "DEBUG_PPROF_ENABLED",
		"HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT",
		"HTTP_READ_HEADER_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT", "HTTP_MAX_HEADER_BYTES",
		"UPSTREAM_TIMEOUT", "UPSTREAM_RETRY_MAX_ATTEMPTS", "UPSTREAM_RETRY_BASE_DELAY",
		"UPSTREAM_RETRY_MAX_DELAY", "REQUEST_BODY_MAX_BYTES", "SIM_CAPABILITIES_TTL",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadRunConfig_DefaultsWhenNoEnvSet(t *testing.T) {
	clearRunConfigEnv(t)

	cfg, err := loadRunConfig()
	if err != nil {
		t.Fatalf("loadRunConfig error = %v", err)
	}
	if cfg.simBaseURL != "http://localhost:8000" {
		t.Errorf("simBaseURL = %q, want default", cfg.simBaseURL)
	}
	if cfg.pollMs != 2000 {
		t.Errorf("pollMs = %d, want default 2000", cfg.pollMs)
	}
	if cfg.swaggerEnabled {
		t.Errorf("swaggerEnabled = true, want default false")
	}
	if cfg.http.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.http.ListenAddr)
	}
	if cfg.upstream.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want default 3", cfg.upstream.MaxAttempts)
	}
	if cfg.simCapTTL != 5*time.Second {
		t.Errorf("simCapTTL = %v, want default 5s", cfg.simCapTTL)
	}
}

func TestLoadRunConfig_ReadsAllEnvOverrides(t *testing.T) {
	clearRunConfigEnv(t)
	t.Setenv("SIMULATOR_BASE_URL", "http://sim.internal:9000/")
	t.Setenv("DASHBOARD_POLL_MS", "500")
	t.Setenv("SWAGGER_ENABLED", "true")
	t.Setenv("SWAGGER_TOKEN", "  secret-token  ")
	t.Setenv("DASHBOARD_PORT", "9090")
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("DEBUG_PPROF_ENABLED", "true")
	t.Setenv("SIM_CAPABILITIES_TTL", "10s")

	cfg, err := loadRunConfig()
	if err != nil {
		t.Fatalf("loadRunConfig error = %v", err)
	}
	if cfg.simBaseURL != "http://sim.internal:9000" {
		t.Errorf("simBaseURL = %q, want trailing slash trimmed", cfg.simBaseURL)
	}
	if cfg.pollMs != 500 {
		t.Errorf("pollMs = %d, want 500", cfg.pollMs)
	}
	if !cfg.swaggerEnabled {
		t.Errorf("swaggerEnabled = false, want true")
	}
	if cfg.swaggerToken != "secret-token" {
		t.Errorf("swaggerToken = %q, want trimmed", cfg.swaggerToken)
	}
	if cfg.http.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want :9090", cfg.http.ListenAddr)
	}
	if !cfg.metricsEnabled || !cfg.pprofEnabled {
		t.Errorf("metricsEnabled/pprofEnabled = %v/%v, want both true", cfg.metricsEnabled, cfg.pprofEnabled)
	}
	if cfg.simCapTTL != 10*time.Second {
		t.Errorf("simCapTTL = %v, want 10s", cfg.simCapTTL)
	}
}

func TestLoadRunConfig_InvalidSimulatorBaseURL_ReturnsError(t *testing.T) {
	clearRunConfigEnv(t)
	t.Setenv("SIMULATOR_BASE_URL", "://not-a-valid-url")

	_, err := loadRunConfig()
	if err == nil {
		t.Fatal("expected error for invalid SIMULATOR_BASE_URL")
	}
	if !strings.Contains(err.Error(), "invalid SIMULATOR_BASE_URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRunConfig_UpstreamRetryMaxAttemptsFloorsAtOne(t *testing.T) {
	clearRunConfigEnv(t)
	t.Setenv("UPSTREAM_RETRY_MAX_ATTEMPTS", "0")

	cfg, err := loadRunConfig()
	if err != nil {
		t.Fatalf("loadRunConfig error = %v", err)
	}
	if cfg.upstream.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts = %d, want floored to 1", cfg.upstream.MaxAttempts)
	}
}
