package geocode

import (
	"context"
	"testing"
	"time"

	"route-optimizer-go/internal/optimizer"
)

type timedGeocodeProvider struct {
	times []time.Time
}

func (p *timedGeocodeProvider) Geocode(context.Context, string) (optimizer.Stop, error) {
	p.times = append(p.times, time.Now())
	return optimizer.Stop{Name: "Resolved", Lat: 40, Lon: -73}, nil
}

func TestRateLimitedSpacesProviderCalls(t *testing.T) {
	provider := &timedGeocodeProvider{}
	limited := NewRateLimited(provider, &Limiter{Interval: 20 * time.Millisecond})

	if _, err := limited.Geocode(context.Background(), "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Geocode(context.Background(), "B"); err != nil {
		t.Fatal(err)
	}
	if len(provider.times) != 2 || provider.times[1].Sub(provider.times[0]) < 15*time.Millisecond {
		t.Fatalf("provider call times = %#v", provider.times)
	}
}

func TestRateLimitedHonorsCanceledWait(t *testing.T) {
	provider := &timedGeocodeProvider{}
	limited := NewRateLimited(provider, &Limiter{Interval: time.Hour})
	if _, err := limited.Geocode(context.Background(), "A"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := limited.Geocode(ctx, "B"); err == nil {
		t.Fatal("Geocode canceled wait error = nil")
	}
	if len(provider.times) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.times))
	}
}

func TestWithPublicNominatimRateLimitOnlyWrapsPublicHost(t *testing.T) {
	provider := &timedGeocodeProvider{}
	if _, ok := WithPublicNominatimRateLimit(provider, "https://nominatim.openstreetmap.org").(*RateLimited); !ok {
		t.Fatal("public Nominatim provider is not rate limited")
	}
	if got := WithPublicNominatimRateLimit(provider, "https://nominatim.example"); got != provider {
		t.Fatal("custom Nominatim provider should not use the public-service limiter")
	}
}
