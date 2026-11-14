// Package config loads runtime settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env            string        // "dev" or "prod"
	HTTPAddr       string        // e.g. ":8080"
	DatabaseURL    string        // postgres://...
	SessionTTL     time.Duration // idle session lifetime
	CookieSecure   bool          // Secure flag on the session cookie
	GotenbergURL   string        // http://gotenberg:3000
	FilesDir       string        // uploads and generated PDFs
	TimeZone       string        // school time zone for due dates and penalties
	MigrateOnStart bool
	RunJobs        bool // daily penalty job in this process
	// FakeToday pins "today" (YYYY-MM-DD) for demos and training environments.
	// Never set it in production.
	FakeToday string
}

// Load reads configuration from the environment, applying defaults suitable
// for local development.
func Load() (Config, error) {
	c := Config{
		Env:            get("APP_ENV", "dev"),
		HTTPAddr:       get("HTTP_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		GotenbergURL:   get("GOTENBERG_URL", "http://localhost:3000"),
		FilesDir:       get("FILES_DIR", "./data/files"),
		TimeZone:       get("APP_TIMEZONE", "Africa/Maputo"),
		MigrateOnStart: getBool("MIGRATE_ON_START", true),
		RunJobs:        getBool("RUN_JOBS", true),
		FakeToday:      os.Getenv("FAKE_TODAY"),
	}
	c.CookieSecure = getBool("COOKIE_SECURE", c.Env == "prod")

	ttl, err := time.ParseDuration(get("SESSION_TTL", "12h"))
	if err != nil {
		return c, fmt.Errorf("SESSION_TTL: %w", err)
	}
	c.SessionTTL = ttl

	if c.FakeToday != "" {
		if _, err := time.Parse(time.DateOnly, c.FakeToday); err != nil {
			return c, fmt.Errorf("FAKE_TODAY: %w", err)
		}
		if c.Env == "prod" {
			return c, fmt.Errorf("FAKE_TODAY must not be set in prod")
		}
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if _, err := time.LoadLocation(c.TimeZone); err != nil {
		return c, fmt.Errorf("APP_TIMEZONE: %w", err)
	}
	return c, nil
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
