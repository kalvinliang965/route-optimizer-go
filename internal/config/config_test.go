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
}
