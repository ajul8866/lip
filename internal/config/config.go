// Package config loads and validates process configuration from the
// environment. Safety-critical settings fail closed: an unreadable or invalid
// safe-local-only configuration resolves to enabled with an empty egress
// allowlist.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAddr           = ":8080"
	DefaultSessionTTL     = 12 * time.Hour
	DefaultMaxConcurrent  = 2
	MaxConcurrentLimit    = 8
	DefaultImportRoot     = "./data/imports"
	DefaultShutdown       = 15 * time.Second
	DefaultMaxRequestSize = 2 << 20 // 2 MiB
)

// Config is the fully resolved runtime configuration.
type Config struct {
	Addr            string
	DatabaseURL     string
	DBMaxConns      int32
	SessionCookie   string
	SessionTTL      time.Duration
	SecureCookies   bool
	TrustProxy      bool
	SafeLocalOnly   bool
	SafeModeSource  string
	EgressAllowlist []string
	ImportRoot      string
	MaxConcurrent   int
	AutoRun         bool
	AutoMigrate     bool
	LogLevel        slog.Level
	LogFormat       string
	DefaultLocale   string
	ShutdownTimeout time.Duration
	MaxRequestBytes int64
	SetupToken      string
	Version         string
	Warnings        []string
}

// Load reads configuration from the process environment.
func Load(version string) (Config, error) {
	return load(os.LookupEnv, version)
}

type lookupFunc func(string) (string, bool)

func load(lookup lookupFunc, version string) (Config, error) {
	cfg := Config{
		Addr:            env(lookup, "LIP_ADDR", DefaultAddr),
		DatabaseURL:     env(lookup, "DATABASE_URL", ""),
		DBMaxConns:      int32(envInt(lookup, "LIP_DB_MAX_CONNS", 10, 1, 100, nil)),
		SessionCookie:   env(lookup, "LIP_SESSION_COOKIE", "lip_session"),
		SessionTTL:      DefaultSessionTTL,
		SecureCookies:   envBool(lookup, "LIP_SECURE_COOKIES", false, nil),
		TrustProxy:      envBool(lookup, "LIP_TRUST_PROXY", false, nil),
		ImportRoot:      env(lookup, "LIP_IMPORT_ROOT", DefaultImportRoot),
		MaxConcurrent:   envInt(lookup, "LIP_MAX_CONCURRENT_TASKS", DefaultMaxConcurrent, 1, MaxConcurrentLimit, nil),
		AutoRun:         envBool(lookup, "LIP_AUTORUN", true, nil),
		AutoMigrate:     envBool(lookup, "LIP_AUTO_MIGRATE", true, nil),
		LogFormat:       strings.ToLower(env(lookup, "LIP_LOG_FORMAT", "json")),
		DefaultLocale:   strings.ToLower(env(lookup, "LIP_DEFAULT_LOCALE", "en")),
		ShutdownTimeout: DefaultShutdown,
		MaxRequestBytes: int64(envInt(lookup, "LIP_MAX_REQUEST_MB", 2, 1, 32, nil)) << 20,
		SetupToken:      env(lookup, "LIP_SETUP_TOKEN", ""),
		Version:         version,
	}
	cfg.LogLevel = parseLogLevel(env(lookup, "LIP_LOG_LEVEL", "info"), &cfg.Warnings)

	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if u, err := url.Parse(cfg.DatabaseURL); err != nil || u.Scheme == "" {
		return cfg, fmt.Errorf("DATABASE_URL is not a valid URL")
	}

	if d, err := parseDuration(lookup, "LIP_SESSION_TTL", DefaultSessionTTL); err != nil {
		return cfg, err
	} else {
		cfg.SessionTTL = d
	}
	if d, err := parseDuration(lookup, "LIP_SHUTDOWN_TIMEOUT", DefaultShutdown); err != nil {
		return cfg, err
	} else {
		cfg.ShutdownTimeout = d
	}

	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		cfg.Warnings = append(cfg.Warnings, "invalid LIP_LOG_FORMAT; using json")
		cfg.LogFormat = "json"
	}
	if cfg.DefaultLocale != "en" && cfg.DefaultLocale != "ko" {
		cfg.Warnings = append(cfg.Warnings, "invalid LIP_DEFAULT_LOCALE; using en")
		cfg.DefaultLocale = "en"
	}

	// Safe local-only: missing means enabled (the secure default). Any value
	// other than an exact true/false is treated as enabled AND clears the
	// allowlist, so a typo cannot open egress.
	cfg.SafeLocalOnly = true
	cfg.SafeModeSource = "default"
	if raw, ok := lookup("LIP_SAFE_LOCAL_ONLY"); ok {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true", "1", "yes":
			cfg.SafeModeSource = "env"
		case "false", "0", "no":
			cfg.SafeLocalOnly = false
			cfg.SafeModeSource = "env"
		default:
			cfg.SafeModeSource = "invalid-fallback"
			cfg.Warnings = append(cfg.Warnings, "invalid LIP_SAFE_LOCAL_ONLY; safe local-only enforced with an empty egress allowlist")
		}
	}

	if raw, ok := lookup("LIP_EGRESS_ALLOWLIST"); ok && cfg.SafeModeSource != "invalid-fallback" {
		cfg.EgressAllowlist = splitList(raw)
	}

	abs, err := filepath.Abs(cfg.ImportRoot)
	if err != nil {
		return cfg, fmt.Errorf("resolve LIP_IMPORT_ROOT: %w", err)
	}
	cfg.ImportRoot = abs

	if cfg.SafeLocalOnly && cfg.SetupToken == "" {
		cfg.Warnings = append(cfg.Warnings, "LIP_SETUP_TOKEN is empty; first-run setup is available to anyone who can reach the API until an admin exists")
	}
	return cfg, nil
}

func env(lookup lookupFunc, key, fallback string) string {
	if v, ok := lookup(key); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return fallback
}

func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseDuration(lookup lookupFunc, key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if d < time.Minute || d > 30*24*time.Hour {
		return 0, fmt.Errorf("%s must be between 1m and 720h", key)
	}
	return d, nil
}

func envBool(lookup lookupFunc, key string, fallback bool, warnings *[]string) bool {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	b, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		if warnings != nil {
			*warnings = append(*warnings, fmt.Sprintf("invalid %s; using %t", key, fallback))
		}
		return fallback
	}
	return b
}

func envInt(lookup lookupFunc, key string, fallback, min, max int, warnings *[]string) int {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < min || n > max {
		if warnings != nil {
			*warnings = append(*warnings, fmt.Sprintf("invalid %s; using %d", key, fallback))
		}
		return fallback
	}
	return n
}

func parseLogLevel(raw string, warnings *[]string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		*warnings = append(*warnings, "invalid LIP_LOG_LEVEL; using info")
		return slog.LevelInfo
	}
}
