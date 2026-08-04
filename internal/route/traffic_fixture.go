package route

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type trafficFixtureFile struct {
	UpdatedAt         string               `yaml:"updated_at"`
	DefaultMultiplier float64              `yaml:"default_multiplier"`
	Edges             []trafficFixtureEdge `yaml:"edges"`
}

type trafficFixtureEdge struct {
	FromStop   int     `yaml:"from_stop"`
	ToStop     int     `yaml:"to_stop"`
	Multiplier float64 `yaml:"multiplier"`
}

// LoadTrafficFixture reads a YAML traffic snapshot used for demos and offline tests.
func LoadTrafficFixture(path string) (TrafficSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TrafficSnapshot{}, fmt.Errorf("read traffic fixture %s: %w", path, err)
	}

	var file trafficFixtureFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return TrafficSnapshot{}, fmt.Errorf("parse traffic fixture %s: %w", path, err)
	}

	if file.DefaultMultiplier <= 0 {
		return TrafficSnapshot{}, fmt.Errorf("traffic fixture %s: default_multiplier must be > 0", path)
	}

	updatedAt := time.Time{}
	if file.UpdatedAt != "" {
		updatedAt, err = time.Parse(time.RFC3339, file.UpdatedAt)
		if err != nil {
			return TrafficSnapshot{}, fmt.Errorf("traffic fixture %s: invalid updated_at: %w", path, err)
		}
	}

	edges := make(map[[2]int]float64, len(file.Edges))
	for i, e := range file.Edges {
		if e.FromStop < 0 || e.ToStop < 0 {
			return TrafficSnapshot{}, fmt.Errorf("traffic fixture %s: edges[%d] has negative stop index", path, i)
		}
		if e.FromStop == e.ToStop {
			return TrafficSnapshot{}, fmt.Errorf("traffic fixture %s: edges[%d] from_stop == to_stop", path, i)
		}
		if e.Multiplier <= 0 {
			return TrafficSnapshot{}, fmt.Errorf("traffic fixture %s: edges[%d] multiplier must be > 0", path, i)
		}
		edges[[2]int{e.FromStop, e.ToStop}] = e.Multiplier
	}

	return TrafficSnapshot{
		UpdatedAt:         updatedAt,
		DefaultMultiplier: file.DefaultMultiplier,
		EdgeMultipliers:   edges,
	}, nil
}
