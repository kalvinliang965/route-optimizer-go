package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Solver SolverConfig `yaml:"solver"`
	Input  InputConfig  `yaml:"input"`
	Cache  CacheConfig  `yaml:"cache"`
	HTTP   HTTPConfig   `yaml:"http"`
	Output OutputConfig `yaml:"output"`
}

type SolverConfig struct {
	TopK      int `yaml:"top_k"`
	MaxStops  int `yaml:"max_stops"`
}

type InputConfig struct {
	AddressesFile string   `yaml:"addresses_file"`
	Addresses     []string `yaml:"addresses"`
}

type CacheConfig struct {
	GeocodeFile string `yaml:"geocode_file"`
	MatrixFile  string `yaml:"matrix_file"`
}

type HTTPConfig struct {
	GeocodeTimeoutSec int    `yaml:"geocode_timeout_sec"`
	OSRMTimeoutSec    int    `yaml:"osrm_timeout_sec"`
	UserAgent         string `yaml:"user_agent"`
}

type OutputConfig struct {
	DurationUnit string `yaml:"duration_unit"` // "minutes" or "seconds"
}

func defaultConfig() Config {
	return Config{
		Solver: SolverConfig{TopK: 5, MaxStops: 15},
		Input:  InputConfig{AddressesFile: "addresses.txt"},
		Cache: CacheConfig{
			GeocodeFile: "data/geocode_cache.json",
			MatrixFile:  "data/matrix_cache.json",
		},
		HTTP: HTTPConfig{
			GeocodeTimeoutSec: 5,
			OSRMTimeoutSec:    10,
			UserAgent:         "GoRouteOptimizerApp/1.0 (student project)",
		},
		Output: OutputConfig{DurationUnit: "minutes"},
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.Solver.TopK < 1 {
		return fmt.Errorf("solver.top_k must be >= 1, got %d", c.Solver.TopK)
	}
	if c.Solver.MaxStops < 1 {
		return fmt.Errorf("solver.max_stops must be >= 1, got %d", c.Solver.MaxStops)
	}
	if c.HTTP.GeocodeTimeoutSec < 1 {
		return fmt.Errorf("http.geocode_timeout_sec must be >= 1, got %d", c.HTTP.GeocodeTimeoutSec)
	}
	if c.HTTP.OSRMTimeoutSec < 1 {
		return fmt.Errorf("http.osrm_timeout_sec must be >= 1, got %d", c.HTTP.OSRMTimeoutSec)
	}
	switch c.Output.DurationUnit {
	case "minutes", "seconds":
	default:
		return fmt.Errorf("output.duration_unit must be \"minutes\" or \"seconds\", got %q", c.Output.DurationUnit)
	}
	return nil
}

func (c Config) applyRuntime() {
	maxStops = c.Solver.MaxStops
	geocodeTimeout = time.Duration(c.HTTP.GeocodeTimeoutSec) * time.Second
	osrmTimeout = time.Duration(c.HTTP.OSRMTimeoutSec) * time.Second
	httpUserAgent = c.HTTP.UserAgent
}

func (c Config) resolveAddresses(cliAddressesFile string) ([]string, error) {
	path := cliAddressesFile
	if path == "" {
		path = c.Input.AddressesFile
	}
	if path != "" {
		return readAddressesFromFile(path)
	}
	if len(c.Input.Addresses) > 0 {
		return append([]string(nil), c.Input.Addresses...), nil
	}
	return nil, fmt.Errorf("no addresses: set input.addresses_file, input.addresses, or pass a file argument")
}
