package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// address string to their resolve stops
type GeocodeCache map[string]Stop;

func loadGeocode(filename string) (GeocodeCache, error) {
	cache := make(GeocodeCache);
	fileBytes, err := os.ReadFile(filename);
	if err != nil {
		return cache, nil; // return empty cache if file dont exists
	}
	err = json.Unmarshal(fileBytes, &cache);
	return cache, err;
}

func saveGeocode(filename string, cache GeocodeCache) error {
	fileBytes, err  := json.MarshalIndent(cache, "", " ");
	if err != nil {
		return err;
	}
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filename, fileBytes, 0644);

}

// matrix cache will map pairs of stop's name to their durations
type MatrixCache map[string]map[string]float64;

func loadMatrix(filename string) (MatrixCache, error) {
	cache := make(MatrixCache);
	fileBytes, err := os.ReadFile(filename);
	if err != nil {
		return cache, nil; // return cache if file dont exists;
	}
	err = json.Unmarshal(fileBytes, &cache);
	return cache, err;
}

func saveMatrix(filename string, cache MatrixCache) error { 
	fileBytes, err := json.MarshalIndent(cache, "", " ");
	if err != nil {
		return err;
	}
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filename, fileBytes, 0644);
}







