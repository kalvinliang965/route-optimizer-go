package route

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ExampleFile(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig(config.example.yaml): %v", err)
	}
	if cfg.Solver.TopK != 5 {
		t.Errorf("TopK = %d; want 5", cfg.Solver.TopK)
	}
	if cfg.Solver.MaxStops != 15 {
		t.Errorf("MaxStops = %d; want 15", cfg.Solver.MaxStops)
	}
	if cfg.Cache.GeocodeFile != "data/geocode_cache.json" {
		t.Errorf("GeocodeFile = %q", cfg.Cache.GeocodeFile)
	}
	if cfg.HTTP.DOTTimeoutSec != 30 {
		t.Errorf("DOTTimeoutSec = %d; want 30", cfg.HTTP.DOTTimeoutSec)
	}
	if cfg.Output.DurationUnit != "minutes" {
		t.Errorf("DurationUnit = %q; want minutes", cfg.Output.DurationUnit)
	}
}

func TestLoadConfig_ValidateTopK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("solver:\n  top_k: 0\n  max_stops: 15\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected validation error for top_k=0")
	}
}

func TestLoadConfig_ValidateDOTTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-dot-timeout.yaml")
	if err := os.WriteFile(path, []byte("http:\n  dot_timeout_sec: 0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected validation error for dot_timeout_sec=0")
	}
}

func TestConfig_ApplyRuntimeSetsMaxStops(t *testing.T) {
	old := MaxStops
	defer func() { MaxStops = old }()

	cfg := defaultConfig()
	cfg.Solver.MaxStops = 7
	cfg.ApplyRuntime()
	if MaxStops != 7 {
		t.Errorf("MaxStops = %d; want 7", MaxStops)
	}
}
