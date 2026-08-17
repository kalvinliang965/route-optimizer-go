package matrix

import (
	"context"
	"testing"
	"time"

	"route-optimizer-go/internal/optimizer"
)

type timedMatrixProvider struct {
	times []time.Time
}

func (p *timedMatrixProvider) Durations(context.Context, []optimizer.Stop) (optimizer.Matrix, error) {
	p.times = append(p.times, time.Now())
	return optimizer.Matrix{{0, 1}, {1, 0}}, nil
}

func TestRateLimitedSpacesProviderCalls(t *testing.T) {
	provider := &timedMatrixProvider{}
	limited := NewRateLimited(provider, &Limiter{Interval: 20 * time.Millisecond})
	stops := []optimizer.Stop{{}, {}}

	if _, err := limited.Durations(context.Background(), stops); err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Durations(context.Background(), stops); err != nil {
		t.Fatal(err)
	}
	if len(provider.times) != 2 || provider.times[1].Sub(provider.times[0]) < 15*time.Millisecond {
		t.Fatalf("provider call times = %#v", provider.times)
	}
}

func TestRateLimitedSpacesConcurrentProviderCalls(t *testing.T) {
	provider := &timedMatrixProvider{}
	limiter := &Limiter{Interval: 20 * time.Millisecond}
	first := NewRateLimited(provider, limiter)
	second := NewRateLimited(provider, limiter)
	stops := []optimizer.Stop{{}, {}}
	start := make(chan struct{})
	errors := make(chan error, 2)

	for _, limited := range []*RateLimited{first, second} {
		limited := limited
		go func() {
			<-start
			_, err := limited.Durations(context.Background(), stops)
			errors <- err
		}()
	}
	close(start)
	for index := 0; index < 2; index++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	if len(provider.times) != 2 || provider.times[1].Sub(provider.times[0]) < 15*time.Millisecond {
		t.Fatalf("concurrent provider call times = %#v", provider.times)
	}
}

func TestRateLimitedHonorsCanceledWait(t *testing.T) {
	provider := &timedMatrixProvider{}
	limited := NewRateLimited(provider, &Limiter{Interval: time.Hour})
	stops := []optimizer.Stop{{}, {}}
	if _, err := limited.Durations(context.Background(), stops); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := limited.Durations(ctx, stops); err == nil {
		t.Fatal("Durations canceled wait error = nil")
	}
	if len(provider.times) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.times))
	}
}

func TestRateLimitedDoesNotReserveSlotForLocalResult(t *testing.T) {
	provider := &timedMatrixProvider{}
	limiter := &Limiter{Interval: time.Hour}
	limited := NewRateLimited(provider, limiter)

	if _, err := limited.Durations(context.Background(), []optimizer.Stop{{}}); err != nil {
		t.Fatal(err)
	}
	limiter.mu.Lock()
	next := limiter.next
	limiter.mu.Unlock()
	if !next.IsZero() {
		t.Fatalf("one-stop matrix reserved a public request slot at %v", next)
	}
}

func TestWithPublicOSRMRateLimitOnlyWrapsAndSharesPublicHost(t *testing.T) {
	firstProvider := &timedMatrixProvider{}
	secondProvider := &timedMatrixProvider{}
	first, ok := WithPublicOSRMRateLimit(firstProvider, "https://router.project-osrm.org").(*RateLimited)
	if !ok {
		t.Fatal("public OSRM provider is not rate limited")
	}
	second, ok := WithPublicOSRMRateLimit(secondProvider, "HTTPS://ROUTER.PROJECT-OSRM.ORG/table").(*RateLimited)
	if !ok {
		t.Fatal("public OSRM provider with uppercase URL is not rate limited")
	}
	if first.Limiter != second.Limiter {
		t.Fatal("public OSRM providers do not share a process-wide limiter")
	}
	if first.Limiter.Interval != time.Second {
		t.Fatalf("public OSRM interval = %v, want 1s", first.Limiter.Interval)
	}
	if got := WithPublicOSRMRateLimit(firstProvider, "https://osrm.example"); got != firstProvider {
		t.Fatal("custom OSRM provider should not use the public-service limiter")
	}
}
