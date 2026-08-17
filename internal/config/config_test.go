package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("planner:\n  default_top_k: 3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.Planner.DefaultTopK != 3 || config.Planner.MaxStops != 10 || config.HTTP.MatrixTimeoutSec != 10 {
		t.Fatalf("config = %#v", config)
	}
	if !config.Cache.Enabled || config.Cache.Directory != "data/cache" || config.Cache.GeocodeTTLHours != 90*24 || config.Cache.MatrixTTLHours != 30*24 {
		t.Fatalf("cache config = %#v", config.Cache)
	}
}

func TestDefaultUsesHTTPSForPublicOSRM(t *testing.T) {
	if got := Default().Providers.OSRMBaseURL; got != "https://router.project-osrm.org" {
		t.Fatalf("providers.osrm_base_url = %q", got)
	}
}

func TestLoadKeepsCacheDefaultsWhenLegacyKeysArePresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "cache:\n  geocode_file: data/geocode_cache.json\n  matrix_file: data/matrix_cache.json\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !config.Cache.Enabled || config.Cache.Directory != "data/cache" || config.Cache.GeocodeTTLHours != 90*24 || config.Cache.MatrixTTLHours != 30*24 {
		t.Fatalf("cache config = %#v", config.Cache)
	}
}

func TestValidateRejectsInvalidEnabledCache(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{name: "geocode TTL", change: func(config *Config) { config.Cache.GeocodeTTLHours = 0 }},
		{name: "matrix TTL", change: func(config *Config) { config.Cache.MatrixTTLHours = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Default()
			test.change(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("Validate error = nil")
			}
		})
	}
}
