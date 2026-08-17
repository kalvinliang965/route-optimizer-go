// Package config owns process configuration. Domain packages do not import it.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Planner   PlannerConfig  `yaml:"planner"`
	Providers ProviderConfig `yaml:"providers"`
	Cache     CacheConfig    `yaml:"cache"`
	HTTP      HTTPConfig     `yaml:"http"`
	Output    OutputConfig   `yaml:"output"`
}

type PlannerConfig struct {
	DefaultTopK int `yaml:"default_top_k"`
	MaxTopK     int `yaml:"max_top_k"`
	MaxStops    int `yaml:"max_stops"`
}

type ProviderConfig struct {
	NominatimBaseURL string `yaml:"nominatim_base_url"`
	OSRMBaseURL      string `yaml:"osrm_base_url"`
}

type CacheConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Directory       string `yaml:"directory"`
	GeocodeTTLHours int    `yaml:"geocode_ttl_hours"`
	MatrixTTLHours  int    `yaml:"matrix_ttl_hours"`
}

type HTTPConfig struct {
	GeocodeTimeoutSec int    `yaml:"geocode_timeout_sec"`
	MatrixTimeoutSec  int    `yaml:"matrix_timeout_sec"`
	UserAgent         string `yaml:"user_agent"`
}

type OutputConfig struct {
	DurationUnit string `yaml:"duration_unit"`
}

func Default() Config {
	return Config{
		Planner: PlannerConfig{DefaultTopK: 5, MaxTopK: 20, MaxStops: 10},
		Providers: ProviderConfig{
			NominatimBaseURL: "https://nominatim.openstreetmap.org",
			OSRMBaseURL:      "https://router.project-osrm.org",
		},
		Cache: CacheConfig{
			Enabled:         true,
			Directory:       "data/cache",
			GeocodeTTLHours: 90 * 24,
			MatrixTTLHours:  30 * 24,
		},
		HTTP: HTTPConfig{
			GeocodeTimeoutSec: 5,
			MatrixTimeoutSec:  10,
			UserAgent:         "GoRouteOptimizerApp/1.0 (student project)",
		},
		Output: OutputConfig{DurationUnit: "minutes"},
	}
}

func Load(path string) (Config, error) {
	config := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if c.Planner.DefaultTopK < 1 {
		return fmt.Errorf("planner.default_top_k must be >= 1")
	}
	if c.Planner.MaxTopK < c.Planner.DefaultTopK {
		return fmt.Errorf("planner.max_top_k must be >= planner.default_top_k")
	}
	if c.Planner.MaxStops < 1 {
		return fmt.Errorf("planner.max_stops must be >= 1")
	}
	if c.Cache.Enabled {
		if c.Cache.Directory == "" {
			return fmt.Errorf("cache.directory is required when cache is enabled")
		}
		if c.Cache.GeocodeTTLHours < 1 {
			return fmt.Errorf("cache.geocode_ttl_hours must be >= 1 when cache is enabled")
		}
		if c.Cache.MatrixTTLHours < 1 {
			return fmt.Errorf("cache.matrix_ttl_hours must be >= 1 when cache is enabled")
		}
	}
	if c.HTTP.GeocodeTimeoutSec < 1 || c.HTTP.MatrixTimeoutSec < 1 {
		return fmt.Errorf("HTTP timeouts must be >= 1 second")
	}
	if c.HTTP.UserAgent == "" {
		return fmt.Errorf("http.user_agent is required")
	}
	if c.Output.DurationUnit != "minutes" && c.Output.DurationUnit != "seconds" {
		return fmt.Errorf("output.duration_unit must be minutes or seconds")
	}
	return nil
}
