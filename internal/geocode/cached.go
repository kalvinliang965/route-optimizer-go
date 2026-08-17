package geocode

import (
	"context"
	"fmt"
	"strings"
	"sync"

	routecache "route-optimizer-go/internal/cache"
	"route-optimizer-go/internal/optimizer"
)

// Cached decorates a geocoder with rebuildable persistent storage. Provider
// failures may use an expired result, while cache I/O failures remain warnings.
type Cached struct {
	Next      Provider
	Store     routecache.GeocodeStore
	Namespace string
	Logf      func(string, ...interface{})

	mu       sync.Mutex
	inFlight map[string]*geocodeCall
}

type geocodeCall struct {
	done           chan struct{}
	stop           optimizer.Stop
	err            error
	leaderCanceled bool
}

func NewCached(next Provider, store routecache.GeocodeStore, namespace string, logf func(string, ...interface{})) *Cached {
	return &Cached{Next: next, Store: store, Namespace: namespace, Logf: logf}
}

func (c *Cached) Geocode(ctx context.Context, address string) (optimizer.Stop, error) {
	if c.Next == nil {
		return optimizer.Stop{}, fmt.Errorf("geocode provider is required")
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return optimizer.Stop{}, fmt.Errorf("address is empty")
	}
	if c.Store == nil {
		return c.Next.Geocode(ctx, address)
	}

	key := routecache.GeocodeKey(c.Namespace, address)
	for {
		call, leader := c.beginCall(key)
		if !leader {
			select {
			case <-ctx.Done():
				return optimizer.Stop{}, ctx.Err()
			case <-call.done:
				if call.err != nil && call.leaderCanceled && ctx.Err() == nil {
					continue
				}
				return call.stop, call.err
			}
		}

		stop, err := c.loadOrFetch(ctx, key, address)
		c.finishCall(key, call, stop, err, ctx.Err() != nil)
		return stop, err
	}
}

func (c *Cached) loadOrFetch(ctx context.Context, key, address string) (optimizer.Stop, error) {
	var stale optimizer.Stop
	cached, state, err := c.Store.Load(ctx, key)
	if err != nil {
		c.logf("geocode cache read warning (%s): %v", shortKey(key), err)
	} else if state != routecache.GeocodeMissing {
		if err := validateResolvedStop(cached); err != nil {
			c.logf("geocode cache validation warning (%s): %v", shortKey(key), err)
			state = routecache.GeocodeMissing
		}
	}

	switch state {
	case routecache.GeocodeFresh:
		c.logf("geocode cache hit (%s)", shortKey(key))
		return cached, nil
	case routecache.GeocodeStale:
		c.logf("geocode cache expired (%s); refreshing", shortKey(key))
		stale = cached
	default:
		c.logf("geocode cache miss (%s)", shortKey(key))
	}

	stop, err := c.Next.Geocode(ctx, address)
	if err != nil {
		if stale.Name != "" && ctx.Err() == nil {
			c.logf("geocode provider warning (%s): %v; using expired cache entry", shortKey(key), err)
			return stale, nil
		}
		return optimizer.Stop{}, err
	}
	if err := validateResolvedStop(stop); err != nil {
		return optimizer.Stop{}, fmt.Errorf("geocode provider returned invalid stop: %w", err)
	}
	stop.ID = ""
	if err := c.Store.Save(ctx, key, stop); err != nil {
		c.logf("geocode cache write warning (%s): %v", shortKey(key), err)
	}
	return stop, nil
}

func (c *Cached) beginCall(key string) (*geocodeCall, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inFlight == nil {
		c.inFlight = make(map[string]*geocodeCall)
	}
	if call, found := c.inFlight[key]; found {
		return call, false
	}
	call := &geocodeCall{done: make(chan struct{})}
	c.inFlight[key] = call
	return call, true
}

func (c *Cached) finishCall(key string, call *geocodeCall, stop optimizer.Stop, err error, leaderCanceled bool) {
	c.mu.Lock()
	call.stop = stop
	call.err = err
	call.leaderCanceled = leaderCanceled
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
