package route

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFixtureSource_LoadsYAML(t *testing.T) {
	snap, err := LoadTrafficFixture(filepath.Join("..", "..", "data", "traffic_fixture.yaml"))
	if err != nil {
		t.Fatalf("LoadTrafficFixture: %v", err)
	}

	if snap.DefaultMultiplier != 1.0 {
		t.Errorf("DefaultMultiplier = %v; want 1.0", snap.DefaultMultiplier)
	}

	wantTime, _ := time.Parse(time.RFC3339, "2026-08-03T12:00:00Z")
	if !snap.UpdatedAt.Equal(wantTime) {
		t.Errorf("UpdatedAt = %v; want %v", snap.UpdatedAt, wantTime)
	}

	if got := snap.EdgeMultipliers[[2]int{0, 1}]; got != 1.8 {
		t.Errorf("edge 0→1 = %v; want 1.8", got)
	}
	if got := snap.EdgeMultipliers[[2]int{1, 5}]; got != 1.7 {
		t.Errorf("edge 1→5 = %v; want 1.7", got)
	}
}

func TestFixtureSource_InvalidFile(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := LoadTrafficFixture(filepath.Join(t.TempDir(), "nope.yaml"))
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("bad default multiplier", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.yaml")
		content := "default_multiplier: 0\nedges: []\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadTrafficFixture(path)
		if err == nil {
			t.Fatal("expected error for default_multiplier <= 0")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "junk.yaml")
		if err := os.WriteFile(path, []byte("{{{not yaml"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadTrafficFixture(path)
		if err == nil {
			t.Fatal("expected error for invalid yaml")
		}
	})
}
