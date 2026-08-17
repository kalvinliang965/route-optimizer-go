package matrix

import (
	"context"
	"fmt"
	"math"
	"sync"

	routecache "route-optimizer-go/internal/cache"
	"route-optimizer-go/internal/optimizer"
)

// Provider supplies a directed duration matrix for an ordered stop list.
type Provider interface {
	Durations(context.Context, []optimizer.Stop) (optimizer.Matrix, error)
}

// Cached decorates a matrix provider with rebuildable persistent storage.
// Cache failures are warnings: they never prevent a provider request or result.
type Cached struct {
	Next      Provider
	Store     routecache.MatrixStore
	Namespace string
	Logf      func(string, ...interface{})

	mu       sync.Mutex
	inFlight map[string]*durationCall
}

type durationCall struct {
	done   chan struct{}
	matrix optimizer.Matrix
	err    error
}

func NewCached(next Provider, store routecache.MatrixStore, namespace string, logf func(string, ...interface{})) *Cached {
	return &Cached{Next: next, Store: store, Namespace: namespace, Logf: logf}
}

func (c *Cached) Durations(ctx context.Context, stops []optimizer.Stop) (optimizer.Matrix, error) {
	if c.Next == nil {
		return nil, fmt.Errorf("matrix provider is required")
	}
	if c.Store == nil {
		return c.Next.Durations(ctx, stops)
	}

	key := routecache.MatrixKey(c.Namespace, stops)
	call, leader := c.beginCall(key)
	if !leader {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-call.done:
			return cloneDurationMatrix(call.matrix), call.err
		}
	}

	matrix, err := c.loadOrFetch(ctx, key, stops)
	c.finishCall(key, call, matrix, err)
	return matrix, err
}

func (c *Cached) loadOrFetch(ctx context.Context, key string, stops []optimizer.Stop) (optimizer.Matrix, error) {
	var stale optimizer.Matrix
	cached, state, err := c.Store.Load(ctx, key)
	if err != nil {
		c.logf("matrix cache read warning (%s): %v", shortKey(key), err)
	} else if state != routecache.MatrixMissing {
		if err := validateCachedMatrix(cached, len(stops)); err != nil {
			c.logf("matrix cache validation warning (%s): %v", shortKey(key), err)
			state = routecache.MatrixMissing
		}
	}

	switch state {
	case routecache.MatrixFresh:
		c.logf("matrix cache hit (%s)", shortKey(key))
		return cached, nil
	case routecache.MatrixStale:
		c.logf("matrix cache expired (%s); refreshing", shortKey(key))
		stale = cached
	default:
		c.logf("matrix cache miss (%s)", shortKey(key))
	}

	matrix, err := c.Next.Durations(ctx, stops)
	if err != nil {
		if stale != nil && ctx.Err() == nil {
			c.logf("matrix provider warning (%s): %v; using expired cache entry", shortKey(key), err)
			return stale, nil
		}
		return nil, err
	}
	if err := c.Store.Save(ctx, key, matrix); err != nil {
		c.logf("matrix cache write warning (%s): %v", shortKey(key), err)
	}
	return matrix, nil
}

func (c *Cached) beginCall(key string) (*durationCall, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inFlight == nil {
		c.inFlight = make(map[string]*durationCall)
	}
	if call, found := c.inFlight[key]; found {
		return call, false
	}
	call := &durationCall{done: make(chan struct{})}
	c.inFlight[key] = call
	return call, true
}

func (c *Cached) finishCall(key string, call *durationCall, matrix optimizer.Matrix, err error) {
	c.mu.Lock()
	call.matrix = cloneDurationMatrix(matrix)
	call.err = err
	delete(c.inFlight, key)
	close(call.done)
	c.mu.Unlock()
}

func (c *Cached) logf(format string, arguments ...interface{}) {
	if c.Logf != nil {
		c.Logf(format, arguments...)
	}
}

func shortKey(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12]
}

func validateCachedMatrix(matrix optimizer.Matrix, stopCount int) error {
	if len(matrix) != stopCount {
		return fmt.Errorf("matrix has %d rows, want %d", len(matrix), stopCount)
	}
	for rowIndex, row := range matrix {
		if len(row) != stopCount {
			return fmt.Errorf("row %d has %d columns, want %d", rowIndex, len(row), stopCount)
		}
		for columnIndex, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return fmt.Errorf("matrix[%d][%d] must be finite and non-negative", rowIndex, columnIndex)
			}
		}
	}
	return nil
}

func cloneDurationMatrix(matrix optimizer.Matrix) optimizer.Matrix {
	cloned := make(optimizer.Matrix, len(matrix))
	for index := range matrix {
		cloned[index] = append([]float64(nil), matrix[index]...)
	}
	return cloned
}
