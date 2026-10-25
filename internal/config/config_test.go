package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) lookupFunc {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestLoadDefaultsFailClosed(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{
		"DATABASE_URL": "postgres://localhost:5432/lip?sslmode=disable",
	}), "test")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.SafeLocalOnly {
		t.Fatal("safe local-only must default to enabled")
	}
	if cfg.SafeModeSource != "default" {
		t.Fatalf("source = %q", cfg.SafeModeSource)
	}
	if len(cfg.EgressAllowlist) != 0 {
		t.Fatalf("allowlist should default empty: %v", cfg.EgressAllowlist)
	}
	if cfg.MaxConcurrent != 2 || !cfg.AutoRun {
		t.Fatalf("unexpected autonomy defaults: %+v", cfg)
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Fatalf("ttl = %v", cfg.SessionTTL)
	}
}

func TestInvalidSafeModeFailsClosed(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{
		"DATABASE_URL":         "postgres://localhost:5432/lip",
		"LIP_SAFE_LOCAL_ONLY":  "definitely-not-a-bool",
		"LIP_EGRESS_ALLOWLIST": "api.example.com:443",
	}), "test")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.SafeLocalOnly {
		t.Fatal("invalid safe mode must resolve to enabled")
	}
	if cfg.SafeModeSource != "invalid-fallback" {
		t.Fatalf("source = %q", cfg.SafeModeSource)
	}
	if len(cfg.EgressAllowlist) != 0 {
		t.Fatalf("invalid safe mode must clear the allowlist: %v", cfg.EgressAllowlist)
	}
	if len(cfg.Warnings) == 0 {
		t.Fatal("expected a warning")
	}
}

func TestExplicitStandardModeKeepsAllowlist(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{
		"DATABASE_URL":             "postgres://localhost:5432/lip",
		"LIP_SAFE_LOCAL_ONLY":      "false",
		"LIP_EGRESS_ALLOWLIST":     "api.example.com:443, 127.0.0.1:11434",
		"LIP_MAX_CONCURRENT_TASKS": "4",
		"LIP_LOG_LEVEL":            "debug",
	}), "test")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.SafeLocalOnly {
		t.Fatal("expected standard mode")
	}
	if len(cfg.EgressAllowlist) != 2 {
		t.Fatalf("allowlist = %v", cfg.EgressAllowlist)
	}
	if cfg.MaxConcurrent != 4 || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsBadDatabaseURL(t *testing.T) {
	if _, err := load(lookupFrom(map[string]string{}), "test"); err == nil {
		t.Fatal("expected missing DATABASE_URL error")
	}
	if _, err := load(lookupFrom(map[string]string{"DATABASE_URL": "not a url"}), "test"); err == nil {
		t.Fatal("expected invalid DATABASE_URL error")
	}
}

func TestDurationBounds(t *testing.T) {
	_, err := load(lookupFrom(map[string]string{
		"DATABASE_URL":    "postgres://localhost/lip",
		"LIP_SESSION_TTL": "30s",
	}), "test")
	if err == nil || !strings.Contains(err.Error(), "LIP_SESSION_TTL") {
		t.Fatalf("expected duration error, got %v", err)
	}
	cfg, err := load(lookupFrom(map[string]string{
		"DATABASE_URL":    "postgres://localhost/lip",
		"LIP_SESSION_TTL": (2 * time.Hour).String(),
	}), "test")
	if err != nil || cfg.SessionTTL != 2*time.Hour {
		t.Fatalf("unexpected: %v %v", cfg.SessionTTL, err)
	}
}
