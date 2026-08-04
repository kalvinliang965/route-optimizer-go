package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ExampleFile(t *testing.T) {
	cfg, err := loadConfig("config.example.yaml")
	if err != nil {
		t.Fatalf("loadConfig(config.example.yaml): %v", err)
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
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected validation error for top_k=0")
	}
}

func TestConfig_ApplyRuntimeSetsMaxStops(t *testing.T) {
	old := maxStops
	defer func() { maxStops = old }()

	cfg := defaultConfig()
	cfg.Solver.MaxStops = 7
	cfg.applyRuntime()
	if maxStops != 7 {
		t.Errorf("maxStops = %d; want 7", maxStops)
	}
}
