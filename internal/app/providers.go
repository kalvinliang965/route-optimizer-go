// Package app contains shared process composition for CLI and server entry
// points. It does not contain transport or domain logic.
package app

import (
	"strings"
	"time"

	routecache "route-optimizer-go/internal/cache"
	"route-optimizer-go/internal/config"
	"route-optimizer-go/internal/geocode"
	"route-optimizer-go/internal/matrix"
	"route-optimizer-go/internal/planner"
)

func NewGeocoder(cfg config.Config, baseURL string, logf func(string, ...interface{})) planner.Geocoder {
	if baseURL == "" {
		baseURL = cfg.Providers.NominatimBaseURL
	}
	provider := geocode.WithPublicNominatimRateLimit(geocode.NewNominatim(
		baseURL,
		cfg.HTTP.UserAgent,
		time.Duration(cfg.HTTP.GeocodeTimeoutSec)*time.Second,
	), baseURL)
	if !cfg.Cache.Enabled {
		return provider
	}

	store := routecache.NewFileGeocodeStore(
		cfg.Cache.Directory,
		time.Duration(cfg.Cache.GeocodeTTLHours)*time.Hour,
	)
	namespace := "nominatim-search:v1:format=json:limit=1:" + strings.TrimRight(baseURL, "/")
	return geocode.NewCached(provider, store, namespace, logf)
}

func NewMatrixProvider(cfg config.Config, baseURL string, logf func(string, ...interface{})) planner.MatrixProvider {
	if baseURL == "" {
		baseURL = cfg.Providers.OSRMBaseURL
	}
	provider := matrix.WithPublicOSRMRateLimit(matrix.NewOSRM(
		baseURL,
		cfg.HTTP.UserAgent,
		time.Duration(cfg.HTTP.MatrixTimeoutSec)*time.Second,
	), baseURL)
	if !cfg.Cache.Enabled {
		return provider
	}

	store := routecache.NewFileMatrixStore(
		cfg.Cache.Directory,
		time.Duration(cfg.Cache.MatrixTTLHours)*time.Hour,
	)
	namespace := "osrm-table:v1:driving:" + strings.TrimRight(baseURL, "/")
	return matrix.NewCached(provider, store, namespace, logf)
}
