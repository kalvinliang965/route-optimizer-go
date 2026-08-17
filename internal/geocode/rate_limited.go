package geocode

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"route-optimizer-go/internal/optimizer"
)

const publicNominatimInterval = time.Second

// Provider is the behavior implemented by geocoders and their decorators.
type Provider interface {
	Geocode(context.Context, string) (optimizer.Stop, error)
}

// Limiter spaces provider call starts by at least Interval. It is safe for
// concurrent use and respects cancellation while waiting for a reserved slot.
type Limiter struct {
	Interval time.Duration

	requestMu sync.Mutex
	mu        sync.Mutex
	next      time.Time
}

func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil || l.Interval <= 0 {
		return nil
	}

	l.mu.Lock()
	now := time.Now()
	start := now
	if l.next.After(start) {
		start = l.next
	}
	l.next = start.Add(l.Interval)
	l.mu.Unlock()

	delay := time.Until(start)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RateLimited applies a shared limiter immediately before provider calls.
type RateLimited struct {
	Next    Provider
	Limiter *Limiter
}

func NewRateLimited(next Provider, limiter *Limiter) *RateLimited {
	return &RateLimited{Next: next, Limiter: limiter}
}

func (r *RateLimited) Geocode(ctx context.Context, address string) (optimizer.Stop, error) {
	if r.Next == nil {
		return optimizer.Stop{}, fmt.Errorf("geocode provider is required")
	}
	if r.Limiter != nil {
		r.Limiter.requestMu.Lock()
		defer r.Limiter.requestMu.Unlock()
	}
	if err := r.Limiter.Wait(ctx); err != nil {
		return optimizer.Stop{}, err
	}
	return r.Next.Geocode(ctx, address)
}

var publicLimiters = struct {
	sync.Mutex
	byHost map[string]*Limiter
}{byHost: make(map[string]*Limiter)}

// WithPublicNominatimRateLimit applies the public service's process-wide
// request pacing. Self-hosted and third-party endpoints are returned unchanged.
func WithPublicNominatimRateLimit(next Provider, baseURL string) Provider {
	host := nominatimHost(baseURL)
	if host != "nominatim.openstreetmap.org" {
		return next
	}

	publicLimiters.Lock()
	limiter := publicLimiters.byHost[host]
	if limiter == nil {
		limiter = &Limiter{Interval: publicNominatimInterval}
		publicLimiters.byHost[host] = limiter
	}
	publicLimiters.Unlock()
	return NewRateLimited(next, limiter)
}

func nominatimHost(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
