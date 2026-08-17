package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"route-optimizer-go/internal/optimizer"
	"route-optimizer-go/internal/storage"
)

const geocodeSchemaVersion = 1

// GeocodeState describes whether an address lookup missed, found a fresh
// result, or found an expired result that may be used as a provider fallback.
type GeocodeState uint8

const (
	GeocodeMissing GeocodeState = iota
	GeocodeFresh
	GeocodeStale
)

// GeocodeStore is the persistence behavior used by a cached geocoder.
type GeocodeStore interface {
	Load(context.Context, string) (optimizer.Stop, GeocodeState, error)
	Save(context.Context, string, optimizer.Stop) error
}

// FileGeocodeStore keeps each resolved address in its own JSON file so entries
// can be replaced atomically and shared by CLI and server processes.
type FileGeocodeStore struct {
	Directory string
	TTL       time.Duration
	Now       func() time.Time
}

type geocodeEntry struct {
	SchemaVersion int            `json:"schema_version"`
	Key           string         `json:"key"`
	CreatedAt     time.Time      `json:"created_at"`
	ExpiresAt     time.Time      `json:"expires_at"`
	Stop          optimizer.Stop `json:"stop"`
}

func NewFileGeocodeStore(directory string, ttl time.Duration) *FileGeocodeStore {
	return &FileGeocodeStore{Directory: directory, TTL: ttl}
}

// GeocodeKey fingerprints the provider namespace and a normalized address.
// Normalization makes capitalization and repeated whitespace cache-equivalent.
func GeocodeKey(namespace, address string) string {
	var source strings.Builder
	source.WriteString("geocode-cache-v1\n")
	source.WriteString(strings.TrimRight(strings.TrimSpace(namespace), "/"))
	source.WriteByte('\n')
	source.WriteString(normalizeAddress(address))
	source.WriteByte('\n')
	sum := sha256.Sum256([]byte(source.String()))
	return hex.EncodeToString(sum[:])
}

func (s *FileGeocodeStore) Load(ctx context.Context, key string) (optimizer.Stop, GeocodeState, error) {
	if err := ctx.Err(); err != nil {
		return optimizer.Stop{}, GeocodeMissing, err
	}
	path, err := s.entryPath(key)
	if err != nil {
		return optimizer.Stop{}, GeocodeMissing, err
	}

	var entry geocodeEntry
	if err := storage.ReadJSON(path, &entry); err != nil {
		if os.IsNotExist(err) {
			return optimizer.Stop{}, GeocodeMissing, nil
		}
		return optimizer.Stop{}, GeocodeMissing, fmt.Errorf("read geocode cache entry: %w", err)
	}
	if entry.SchemaVersion != geocodeSchemaVersion {
		return optimizer.Stop{}, GeocodeMissing, fmt.Errorf("geocode cache schema %d is unsupported", entry.SchemaVersion)
	}
	if entry.Key != key {
		return optimizer.Stop{}, GeocodeMissing, fmt.Errorf("geocode cache key mismatch")
	}
	if err := validateStop(entry.Stop); err != nil {
		return optimizer.Stop{}, GeocodeMissing, fmt.Errorf("invalid cached stop: %w", err)
	}
	entry.Stop.ID = ""
	if entry.ExpiresAt.IsZero() || !s.now().Before(entry.ExpiresAt) {
		return entry.Stop, GeocodeStale, nil
	}
	return entry.Stop, GeocodeFresh, nil
}

func (s *FileGeocodeStore) Save(ctx context.Context, key string, stop optimizer.Stop) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.TTL <= 0 {
		return fmt.Errorf("geocode cache TTL must be positive")
	}
	if err := validateStop(stop); err != nil {
		return fmt.Errorf("invalid stop: %w", err)
	}
	// IDs identify positions in one request, not the provider location, so they
	// must not leak from one cached batch into another.
	stop.ID = ""
	path, err := s.entryPath(key)
	if err != nil {
		return err
	}

	now := s.now()
	entry := geocodeEntry{
		SchemaVersion: geocodeSchemaVersion,
		Key:           key,
		CreatedAt:     now,
		ExpiresAt:     now.Add(s.TTL),
		Stop:          stop,
	}
	if err := storage.WriteJSON(path, entry); err != nil {
		return fmt.Errorf("write geocode cache entry: %w", err)
	}
	return nil
}

func (s *FileGeocodeStore) entryPath(key string) (string, error) {
	if strings.TrimSpace(s.Directory) == "" {
		return "", fmt.Errorf("geocode cache directory is required")
	}
	if !validSHA256Key(key) {
		return "", fmt.Errorf("invalid geocode cache key")
	}
	return filepath.Join(s.Directory, "geocode", key+".json"), nil
}

func (s *FileGeocodeStore) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeAddress(address string) string {
	return strings.ToLower(strings.Join(strings.Fields(address), " "))
}

func validateStop(stop optimizer.Stop) error {
	if strings.TrimSpace(stop.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if math.IsNaN(stop.Lat) || math.IsInf(stop.Lat, 0) || stop.Lat < -90 || stop.Lat > 90 {
		return fmt.Errorf("latitude must be finite and between -90 and 90")
	}
	if math.IsNaN(stop.Lon) || math.IsInf(stop.Lon, 0) || stop.Lon < -180 || stop.Lon > 180 {
		return fmt.Errorf("longitude must be finite and between -180 and 180")
	}
	return nil
}
