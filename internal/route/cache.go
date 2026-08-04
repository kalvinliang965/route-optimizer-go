package route

import "os"

// address string to their resolve stops
type GeocodeCache map[string]Stop

func LoadGeocode(filename string) (GeocodeCache, error) {
	cache := make(GeocodeCache)
	if err := ReadJSON(filename, &cache); err != nil {
		if os.IsNotExist(err) {
			return cache, nil // empty cache if file doesn't exist
		}
		return nil, err
	}
	return cache, nil
}

func SaveGeocode(filename string, cache GeocodeCache) error {
	return WriteJSON(filename, cache)
}

// matrix cache will map pairs of stop coords to their durations
type MatrixCache map[string]map[string]float64

func LoadMatrix(filename string) (MatrixCache, error) {
	cache := make(MatrixCache)
	if err := ReadJSON(filename, &cache); err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return nil, err
	}
	return cache, nil
}

func SaveMatrix(filename string, cache MatrixCache) error {
	return WriteJSON(filename, cache)
}
