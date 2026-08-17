package geocode

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	routecache "route-optimizer-go/internal/cache"
	"route-optimizer-go/internal/optimizer"
)

type memoryGeocodeStore struct {
	stop  optimizer.Stop
	state routecache.GeocodeState
	err   error
	saves int
}

func (s *memoryGeocodeStore) Load(context.Context, string) (optimizer.Stop, routecache.GeocodeState, error) {
	return s.stop, s.state, s.err
}

func (s *memoryGeocodeStore) Save(_ context.Context, _ string, stop optimizer.Stop) error {
	s.saves++
	s.stop = stop
	s.state = routecache.GeocodeFresh
	return s.err
}

type countingGeocodeProvider struct {
	stop  optimizer.Stop
	err   error
	calls int
}

func (p *countingGeocodeProvider) Geocode(context.Context, string) (optimizer.Stop, error) {
	p.calls++
	return p.stop, p.err
}

func TestCachedUsesStoredGeocode(t *testing.T) {
	want := optimizer.Stop{Name: "Times Square", Lat: 40.757, Lon: -73.986}
	store := &memoryGeocodeStore{stop: want, state: routecache.GeocodeFresh}
	provider := &countingGeocodeProvider{}
	cached := NewCached(provider, store, "nominatim", nil)

	got, err := cached.Geocode(context.Background(), "Times Square")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if got != want || provider.calls != 0 {
		t.Fatalf("stop = %#v, provider calls = %d", got, provider.calls)
	}
}

func TestCachedFetchesAndStoresMiss(t *testing.T) {
	want := optimizer.Stop{Name: "Times Square", Lat: 40.757, Lon: -73.986}
	store := &memoryGeocodeStore{}
	provider := &countingGeocodeProvider{stop: want}
	cached := NewCached(provider, store, "nominatim", nil)

	got, err := cached.Geocode(context.Background(), "Times Square")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if got != want || provider.calls != 1 || store.saves != 1 {
		t.Fatalf("stop = %#v, provider calls = %d, saves = %d", got, provider.calls, store.saves)
	}
}

func TestCachedUsesStaleGeocodeWhenProviderFails(t *testing.T) {
	want := optimizer.Stop{Name: "Times Square", Lat: 40.757, Lon: -73.986}
	store := &memoryGeocodeStore{stop: want, state: routecache.GeocodeStale}
	provider := &countingGeocodeProvider{err: errors.New("Nominatim unavailable")}
	cached := NewCached(provider, store, "nominatim", nil)

	got, err := cached.Geocode(context.Background(), "Times Square")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if got != want || provider.calls != 1 || store.saves != 0 {
		t.Fatalf("stop = %#v, provider calls = %d, saves = %d", got, provider.calls, store.saves)
	}
}

func TestCachedDoesNotStoreProviderFailure(t *testing.T) {
	store := &memoryGeocodeStore{}
	provider := &countingGeocodeProvider{err: errors.New("no result")}
	cached := NewCached(provider, store, "nominatim", nil)

	if _, err := cached.Geocode(context.Background(), "Unknown"); err == nil {
		t.Fatal("Geocode error = nil")
	}
	if store.saves != 0 {
		t.Fatalf("saves = %d, want 0", store.saves)
	}
}

type blockingGeocodeProvider struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (p *blockingGeocodeProvider) Geocode(context.Context, string) (optimizer.Stop, error) {
	p.mu.Lock()
	p.calls++
	if p.calls == 1 {
		close(p.started)
	}
	p.mu.Unlock()
	<-p.release
	return optimizer.Stop{Name: "Times Square", Lat: 40.757, Lon: -73.986}, nil
}

func (p *blockingGeocodeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestCachedCoalescesEquivalentConcurrentAddresses(t *testing.T) {
	store := &memoryGeocodeStore{}
	provider := &blockingGeocodeProvider{started: make(chan struct{}), release: make(chan struct{})}
	cached := NewCached(provider, store, "nominatim", nil)
	addresses := []string{"Times Square", " times  square ", "TIMES SQUARE"}

	start := make(chan struct{})
	results := make(chan error, len(addresses))
	var ready sync.WaitGroup
	ready.Add(len(addresses))
	for _, address := range addresses {
		address := address
		go func() {
			ready.Done()
			<-start
			_, err := cached.Geocode(context.Background(), address)
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	<-provider.started
	close(provider.release)

	for range addresses {
		if err := <-results; err != nil {
			t.Fatalf("Geocode: %v", err)
		}
	}
	if got := provider.callCount(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
}

type cancelThenSucceedProvider struct {
	mu           sync.Mutex
	calls        int
	firstStarted chan struct{}
}

func (p *cancelThenSucceedProvider) Geocode(ctx context.Context, _ string) (optimizer.Stop, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	if call == 1 {
		close(p.firstStarted)
	}
	p.mu.Unlock()
	if call == 1 {
		<-ctx.Done()
		return optimizer.Stop{}, ctx.Err()
	}
	return optimizer.Stop{Name: "Times Square", Lat: 40.757, Lon: -73.986}, nil
}

func (p *cancelThenSucceedProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestCachedFollowerRetriesAfterLeaderCancellation(t *testing.T) {
	provider := &cancelThenSucceedProvider{firstStarted: make(chan struct{})}
	cached := NewCached(provider, &memoryGeocodeStore{}, "nominatim", nil)
	leaderContext, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, err := cached.Geocode(leaderContext, "Times Square")
		leaderResult <- err
	}()
	<-provider.firstStarted

	followerResult := make(chan error, 1)
	go func() {
		_, err := cached.Geocode(context.Background(), "Times Square")
		followerResult <- err
	}()
	// Give the follower time to join the in-flight call before canceling its
	// leader. It must retry with its own live context instead of inheriting the
	// canceled result.
	time.Sleep(10 * time.Millisecond)
	cancelLeader()

	if err := <-leaderResult; err == nil {
		t.Fatal("leader Geocode error = nil")
	}
	if err := <-followerResult; err != nil {
		t.Fatalf("follower Geocode: %v", err)
	}
	if got := provider.callCount(); got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
}
