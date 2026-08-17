package matrix

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"route-optimizer-go/internal/optimizer"
)

const publicOSRMInterval = time.Second

// Limiter spaces provider call starts by at least Interval. It is safe for
// concurrent use and respects cancellation while waiting for a reserved slot.
type Limiter struct {
	Interval time.Duration

	requestMu sync.Mutex
	mu        sync.Mutex
	next      time.Time
}

func (l *Limiter) wait(ctx context.Context) error {
	if l == nil || l.Interval <= 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
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

func (r *RateLimited) Durations(ctx context.Context, stops []optimizer.Stop) (optimizer.Matrix, error) {
	if r.Next == nil {
		return nil, fmt.Errorf("matrix provider is required")
	}
	// OSRM returns these cases locally, without making an HTTP request.
	if len(stops) <= 1 || r.Limiter == nil {
		return r.Next.Durations(ctx, stops)
	}

	r.Limiter.requestMu.Lock()
	defer r.Limiter.requestMu.Unlock()
	if err := r.Limiter.wait(ctx); err != nil {
		return nil, err
	}
	return r.Next.Durations(ctx, stops)
}

var publicOSRMLimiters = struct {
	sync.Mutex
	byHost map[string]*Limiter
}{byHost: make(map[string]*Limiter)}

// WithPublicOSRMRateLimit applies process-wide request pacing to the public
// OSRM demo service. Self-hosted and third-party endpoints are unchanged.
func WithPublicOSRMRateLimit(next Provider, baseURL string) Provider {
	host := osrmHost(baseURL)
	if host != "router.project-osrm.org" {
		return next
	}

	publicOSRMLimiters.Lock()
	limiter := publicOSRMLimiters.byHost[host]
	if limiter == nil {
		limiter = &Limiter{Interval: publicOSRMInterval}
		publicOSRMLimiters.byHost[host] = limiter
	}
	publicOSRMLimiters.Unlock()
	return NewRateLimited(next, limiter)
}

func osrmHost(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
