package main;

import (
	"path/filepath"
	"reflect"
	"testing"
)



func TestGeocodeCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	filepath:= filepath.Join(tmpDir, "geocode_test.json")

	originalCache := GeocodeCache {
		"123 Main St": {Lat: 40.712775, Lon: -73.985428},
		"456 Broad St": {Lat: 40.720000, Lon: -73.990000},
	}

	err := saveGeocode(filepath, originalCache);
	if err != nil {
		t.Fatalf("Failed to save geocode cache: %v", err)
	}

	loadedCache, err := loadGeocode(filepath)
	if err != nil {
		t.Fatalf("Failed to load geocode cache: %v", err)
	}

	if !reflect.DeepEqual(originalCache, loadedCache) {
		t.Errorf("loaded cache codes not match original. \nGot: %v\nWant: %v\n", loadedCache, originalCache);
	}
}

func TestLoadGeocode_FileDoesNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "does_not_exist.json")

	// Should return an empty cache and NO error (based on your code design)
	cache, err := loadGeocode(nonExistentPath)
	if err != nil {
		t.Errorf("expected no error for missing file, got %v", err)
	}

	if len(cache) != 0 {
		t.Errorf("expected empty cache, got %d items", len(cache))
	}
}


func TestMatrixCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "matrix_test.json")

	originalMatrix := MatrixCache{
		"StopA": {
			"StopB": 150.5,
			"StopC": 300.0,
		},
		"StopB": {
			"StopA": 145.2,
		},
	}

	err := saveMatrix(filePath, originalMatrix)
	if err != nil {
		t.Fatalf("failed to save matrix cache: %v", err)
	}

	loadedMatrix, err := loadMatrix(filePath)
	if err != nil {
		t.Fatalf("failed to load matrix cache: %v", err)
	}

	if !reflect.DeepEqual(originalMatrix, loadedMatrix) {
		t.Errorf("loaded matrix does not match original.\nGot:  %v\nWant: %v", loadedMatrix, originalMatrix)
	}
}

func TestLoadMatrix_FileDoesNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "missing_matrix.json")

	cache, err := loadMatrix(nonExistentPath)
	if err != nil {
		t.Errorf("expected no error for missing matrix file, got %v", err)
	}

	if len(cache) != 0 {
		t.Errorf("expected empty matrix cache, got %d items", len(cache))
	}
}