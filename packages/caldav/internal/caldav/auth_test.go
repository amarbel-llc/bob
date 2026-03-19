package caldav

import (
	"os"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	// Save and restore env
	origURL := os.Getenv("CALDAV_URL")
	origUser := os.Getenv("CALDAV_USERNAME")
	origPass := os.Getenv("CALDAV_PASSWORD")
	defer func() {
		os.Setenv("CALDAV_URL", origURL)
		os.Setenv("CALDAV_USERNAME", origUser)
		os.Setenv("CALDAV_PASSWORD", origPass)
	}()

	t.Run("all set", func(t *testing.T) {
		os.Setenv("CALDAV_URL", "https://dav.example.com")
		os.Setenv("CALDAV_USERNAME", "user")
		os.Setenv("CALDAV_PASSWORD", "pass")

		cfg, err := ConfigFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.URL != "https://dav.example.com" {
			t.Errorf("URL = %q", cfg.URL)
		}
		if cfg.Username != "user" {
			t.Errorf("Username = %q", cfg.Username)
		}
		if cfg.Password != "pass" {
			t.Errorf("Password = %q", cfg.Password)
		}
	})

	t.Run("missing URL", func(t *testing.T) {
		os.Setenv("CALDAV_URL", "")
		os.Setenv("CALDAV_USERNAME", "user")
		os.Setenv("CALDAV_PASSWORD", "pass")

		_, err := ConfigFromEnv()
		if err == nil {
			t.Fatal("expected error for missing URL")
		}
	})

	t.Run("missing username", func(t *testing.T) {
		os.Setenv("CALDAV_URL", "https://dav.example.com")
		os.Setenv("CALDAV_USERNAME", "")
		os.Setenv("CALDAV_PASSWORD", "pass")

		_, err := ConfigFromEnv()
		if err == nil {
			t.Fatal("expected error for missing username")
		}
	})

	t.Run("missing password", func(t *testing.T) {
		os.Setenv("CALDAV_URL", "https://dav.example.com")
		os.Setenv("CALDAV_USERNAME", "user")
		os.Setenv("CALDAV_PASSWORD", "")

		_, err := ConfigFromEnv()
		if err == nil {
			t.Fatal("expected error for missing password")
		}
	})
}
