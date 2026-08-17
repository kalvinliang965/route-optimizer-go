package matrix

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	routecache "route-optimizer-go/internal/cache"
	"route-optimizer-go/internal/optimizer"
)

type memoryMatrixStore struct {
	matrix optimizer.Matrix
	state  routecache.MatrixState
	err    error
	saves  int
}

func (s *memoryMatrixStore) Load(context.Context, string) (optimizer.Matrix, routecache.MatrixState, error) {
	return s.matrix, s.state, s.err
}

func (s *memoryMatrixStore) Save(_ context.Context, _ string, matrix optimizer.Matrix) error {
	s.saves++
	s.matrix = matrix
	s.state = routecache.MatrixFresh
	return s.err
}

type countingMatrixProvider struct {
	matrix optimizer.Matrix
	err    error
	calls  int
}

func (p *countingMatrixProvider) Durations(context.Context, []optimizer.Stop) (optimizer.Matrix, error) {
	p.calls++
	return p.matrix, p.err
}

func TestCachedUsesStoredMatrix(t *testing.T) {
	store := &memoryMatrixStore{matrix: optimizer.Matrix{{0, 10}, {12, 0}}, state: routecache.MatrixFresh}
	provider := &countingMatrixProvider{}
	cached := NewCached(provider, store, "osrm:driving", nil)

	got, err := cached.Durations(context.Background(), []optimizer.Stop{{}, {}})
	if err != nil {
		t.Fatalf("Durations: %v", err)
	}
	if got[0][1] != 10 || provider.calls != 0 {
		t.Fatalf("matrix = %#v, provider calls = %d", got, provider.calls)
	}
}

func TestCachedHitBypassesRateLimiter(t *testing.T) {
	store := &memoryMatrixStore{matrix: optimizer.Matrix{{0, 10}, {12, 0}}, state: routecache.MatrixFresh}
	provider := &timedMatrixProvider{}
	limiter := &Limiter{Interval: time.Hour}
	cached := NewCached(NewRateLimited(provider, limiter), store, "osrm:driving", nil)

	if _, err := cached.Durations(context.Background(), []optimizer.Stop{{}, {}}); err != nil {
		t.Fatalf("Durations: %v", err)
	}
	if len(provider.times) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(provider.times))
	}
	limiter.mu.Lock()
	next := limiter.next
	limiter.mu.Unlock()
	if !next.IsZero() {
		t.Fatalf("cache hit reserved a public request slot at %v", next)
	}
}

func TestCachedFetchesAndStoresMiss(t *testing.T) {
	store := &memoryMatrixStore{}
	provider := &countingMatrixProvider{matrix: optimizer.Matrix{{0, 20}, {25, 0}}}
	cached := NewCached(provider, store, "osrm:driving", nil)

	got, err := cached.Durations(context.Background(), []optimizer.Stop{{}, {}})
	if err != nil {
		t.Fatalf("Durations: %v", err)
	}
	if got[0][1] != 20 || provider.calls != 1 || store.saves != 1 {
		t.Fatalf("matrix = %#v, provider calls = %d, saves = %d", got, provider.calls, store.saves)
	}
}

func TestCachedDoesNotStoreProviderFailure(t *testing.T) {
	store := &memoryMatrixStore{}
	provider := &countingMatrixProvider{err: errors.New("OSRM unavailable")}
	cached := NewCached(provider, store, "osrm:driving", nil)

	if _, err := cached.Durations(context.Background(), []optimizer.Stop{{}, {}}); err == nil {
		t.Fatal("Durations error = nil")
	}
	if store.saves != 0 {
		t.Fatalf("saves = %d, want 0", store.saves)
	}
}

func TestCachedUsesStaleMatrixWhenProviderFails(t *testing.T) {
	store := &memoryMatrixStore{
		matrix: optimizer.Matrix{{0, 30}, {35, 0}},
		state:  routecache.MatrixStale,
	}
	provider := &countingMatrixProvider{err: errors.New("OSRM unavailable")}
	cached := NewCached(provider, store, "osrm:driving", nil)

	got, err := cached.Durations(context.Background(), []optimizer.Stop{{}, {}})
	if err != nil {
		t.Fatalf("Durations: %v", err)
	}
	if got[0][1] != 30 || provider.calls != 1 || store.saves != 0 {
		t.Fatalf("matrix = %#v, provider calls = %d, saves = %d", got, provider.calls, store.saves)
	}
}

type blockingMatrixProvider struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (p *blockingMatrixProvider) Durations(context.Context, []optimizer.Stop) (optimizer.Matrix, error) {
	p.mu.Lock()
	p.calls++
	if p.calls == 1 {
		close(p.started)
	}
	p.mu.Unlock()
	<-p.release
	return optimizer.Matrix{{0, 40}, {45, 0}}, nil
}

func (p *blockingMatrixProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestCachedCoalescesConcurrentMisses(t *testing.T) {
	store := &memoryMatrixStore{}
	provider := &blockingMatrixProvider{started: make(chan struct{}), release: make(chan struct{})}
	cached := NewCached(provider, store, "osrm:driving", nil)
	stops := []optimizer.Stop{{}, {}}

	const requests = 20
	start := make(chan struct{})
	results := make(chan error, requests)
	var ready sync.WaitGroup
	ready.Add(requests)
	for index := 0; index < requests; index++ {
		go func() {
			ready.Done()
			<-start
			_, err := cached.Durations(context.Background(), stops)
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	<-provider.started
	close(provider.release)

	for index := 0; index < requests; index++ {
		if err := <-results; err != nil {
			t.Fatalf("Durations: %v", err)
		}
	}
	if got := provider.callCount(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
}
