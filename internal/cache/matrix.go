// Package cache contains rebuildable, persistent provider caches.
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

const matrixSchemaVersion = 1

// MatrixState describes whether a cache lookup missed, found a usable entry,
// or found an expired entry that may be used only as a provider fallback.
type MatrixState uint8

const (
	MatrixMissing MatrixState = iota
	MatrixFresh
	MatrixStale
)

// MatrixStore is the persistence behavior used by a cached matrix provider.
type MatrixStore interface {
	Load(context.Context, string) (optimizer.Matrix, MatrixState, error)
	Save(context.Context, string, optimizer.Matrix) error
}

// FileMatrixStore keeps each complete matrix in its own JSON file. Atomic file
// replacement makes completed entries safe to share between CLI and server
// processes on one machine.
type FileMatrixStore struct {
	Directory string
	TTL       time.Duration
	Now       func() time.Time
}

type matrixEntry struct {
	SchemaVersion int              `json:"schema_version"`
	Key           string           `json:"key"`
	CreatedAt     time.Time        `json:"created_at"`
	ExpiresAt     time.Time        `json:"expires_at"`
	Matrix        optimizer.Matrix `json:"matrix"`
}

func NewFileMatrixStore(directory string, ttl time.Duration) *FileMatrixStore {
	return &FileMatrixStore{Directory: directory, TTL: ttl}
}

// MatrixKey fingerprints the provider namespace and ordered coordinates. Names
// and IDs are intentionally excluded because they do not affect OSRM results.
func MatrixKey(namespace string, stops []optimizer.Stop) string {
	var source strings.Builder
	source.WriteString("matrix-cache-v1\n")
	source.WriteString(strings.TrimRight(strings.TrimSpace(namespace), "/"))
	source.WriteByte('\n')
	for _, stop := range stops {
		_, _ = fmt.Fprintf(&source, "%.6f,%.6f\n", stop.Lon, stop.Lat)
	}
	sum := sha256.Sum256([]byte(source.String()))
	return hex.EncodeToString(sum[:])
}

func (s *FileMatrixStore) Load(ctx context.Context, key string) (optimizer.Matrix, MatrixState, error) {
	if err := ctx.Err(); err != nil {
		return nil, MatrixMissing, err
	}
	path, err := s.entryPath(key)
	if err != nil {
		return nil, MatrixMissing, err
	}

	var entry matrixEntry
	if err := storage.ReadJSON(path, &entry); err != nil {
		if os.IsNotExist(err) {
			return nil, MatrixMissing, nil
		}
		return nil, MatrixMissing, fmt.Errorf("read matrix cache entry: %w", err)
	}
	if entry.SchemaVersion != matrixSchemaVersion {
		return nil, MatrixMissing, fmt.Errorf("matrix cache schema %d is unsupported", entry.SchemaVersion)
	}
	if entry.Key != key {
		return nil, MatrixMissing, fmt.Errorf("matrix cache key mismatch")
	}
	if err := validateMatrix(entry.Matrix); err != nil {
		return nil, MatrixMissing, fmt.Errorf("invalid cached matrix: %w", err)
	}
	if entry.ExpiresAt.IsZero() || !s.now().Before(entry.ExpiresAt) {
		return cloneMatrix(entry.Matrix), MatrixStale, nil
	}
	return cloneMatrix(entry.Matrix), MatrixFresh, nil
}

func (s *FileMatrixStore) Save(ctx context.Context, key string, matrix optimizer.Matrix) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.TTL <= 0 {
		return fmt.Errorf("matrix cache TTL must be positive")
	}
	if err := validateMatrix(matrix); err != nil {
		return fmt.Errorf("invalid matrix: %w", err)
	}
	path, err := s.entryPath(key)
	if err != nil {
		return err
	}

	now := s.now()
	entry := matrixEntry{
		SchemaVersion: matrixSchemaVersion,
		Key:           key,
		CreatedAt:     now,
		ExpiresAt:     now.Add(s.TTL),
		Matrix:        cloneMatrix(matrix),
	}
	if err := storage.WriteJSON(path, entry); err != nil {
		return fmt.Errorf("write matrix cache entry: %w", err)
	}
	return nil
}

func (s *FileMatrixStore) entryPath(key string) (string, error) {
	if strings.TrimSpace(s.Directory) == "" {
		return "", fmt.Errorf("matrix cache directory is required")
	}
	if !validSHA256Key(key) {
		return "", fmt.Errorf("invalid matrix cache key")
	}
	return filepath.Join(s.Directory, "matrices", key+".json"), nil
}

func (s *FileMatrixStore) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func validSHA256Key(key string) bool {
	if len(key) != sha256.Size*2 {
		return false
	}
	for _, character := range key {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func cloneMatrix(matrix optimizer.Matrix) optimizer.Matrix {
	cloned := make(optimizer.Matrix, len(matrix))
	for index := range matrix {
		cloned[index] = append([]float64(nil), matrix[index]...)
	}
	return cloned
}

func validateMatrix(matrix optimizer.Matrix) error {
	if len(matrix) == 0 {
		return fmt.Errorf("matrix must not be empty")
	}
	for rowIndex, row := range matrix {
		if len(row) != len(matrix) {
			return fmt.Errorf("row %d has %d columns, want %d", rowIndex, len(row), len(matrix))
		}
		for columnIndex, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return fmt.Errorf("matrix[%d][%d] must be finite and non-negative", rowIndex, columnIndex)
			}
		}
	}
	return nil
}
