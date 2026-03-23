package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear relevant env vars to test defaults.
	for _, k := range []string{"GITHUB_TOKEN", "FACTORY_DATA_DIR", "NOVELTY_THRESHOLD", "LISTEN_ADDR", "DB_PATH"} {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.GitHubToken != "" {
		t.Errorf("expected empty GitHubToken, got %q", cfg.GitHubToken)
	}
	if cfg.FactoryDataDir != "/tmp/factory-data" {
		t.Errorf("expected /tmp/factory-data, got %q", cfg.FactoryDataDir)
	}
	if cfg.NoveltyThreshold != 0.6 {
		t.Errorf("expected 0.6, got %f", cfg.NoveltyThreshold)
	}
	if cfg.ListenAddr != ":8090" {
		t.Errorf("expected :8090, got %q", cfg.ListenAddr)
	}
	if cfg.DBPath != "/tmp/factory-data/market-scanner.db" {
		t.Errorf("expected /tmp/factory-data/market-scanner.db, got %q", cfg.DBPath)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Setenv("FACTORY_DATA_DIR", "/custom/dir")
	t.Setenv("NOVELTY_THRESHOLD", "0.75")
	t.Setenv("LISTEN_ADDR", ":9090")
	t.Setenv("DB_PATH", "/custom/dir/custom.db")

	cfg := Load()

	if cfg.GitHubToken != "ghp_test123" {
		t.Errorf("expected ghp_test123, got %q", cfg.GitHubToken)
	}
	if cfg.FactoryDataDir != "/custom/dir" {
		t.Errorf("expected /custom/dir, got %q", cfg.FactoryDataDir)
	}
	if cfg.NoveltyThreshold != 0.75 {
		t.Errorf("expected 0.75, got %f", cfg.NoveltyThreshold)
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("expected :9090, got %q", cfg.ListenAddr)
	}
	if cfg.DBPath != "/custom/dir/custom.db" {
		t.Errorf("expected /custom/dir/custom.db, got %q", cfg.DBPath)
	}
}

func TestLoadBadNoveltyThreshold(t *testing.T) {
	t.Setenv("NOVELTY_THRESHOLD", "not-a-number")

	cfg := Load()

	if cfg.NoveltyThreshold != 0.6 {
		t.Errorf("expected fallback 0.6, got %f", cfg.NoveltyThreshold)
	}
}

func TestDBPathDerived(t *testing.T) {
	// When DB_PATH is not set, it should derive from FACTORY_DATA_DIR.
	t.Setenv("FACTORY_DATA_DIR", "/my/data")
	t.Setenv("DB_PATH", "")

	cfg := Load()

	if cfg.DBPath != "/my/data/market-scanner.db" {
		t.Errorf("expected /my/data/market-scanner.db, got %q", cfg.DBPath)
	}
}

func TestEnvOrDefault(t *testing.T) {
	key := "TEST_ENV_OR_DEFAULT_KEY_XYZZY"
	os.Unsetenv(key)

	if v := envOrDefault(key, "fallback"); v != "fallback" {
		t.Errorf("expected fallback, got %q", v)
	}

	os.Setenv(key, "override")
	defer os.Unsetenv(key)

	if v := envOrDefault(key, "fallback"); v != "override" {
		t.Errorf("expected override, got %q", v)
	}
}

func TestEnvFloatOrDefault(t *testing.T) {
	key := "TEST_FLOAT_KEY_XYZZY"

	os.Unsetenv(key)
	if v := envFloatOrDefault(key, 1.5); v != 1.5 {
		t.Errorf("expected 1.5, got %f", v)
	}

	os.Setenv(key, "2.5")
	defer os.Unsetenv(key)
	if v := envFloatOrDefault(key, 1.5); v != 2.5 {
		t.Errorf("expected 2.5, got %f", v)
	}

	os.Setenv(key, "garbage")
	if v := envFloatOrDefault(key, 1.5); v != 1.5 {
		t.Errorf("expected fallback 1.5, got %f", v)
	}
}
