package pepsi

import (
	"context"
	"fmt"
	"time"

	"route-optimizer-go/internal/route"
)

type EdgeTrafficState struct {
	HasData            bool
	CurrentMultiplier  float64
	PreviousMultiplier float64
	HasPrevious        bool
}

type TrafficOptions struct {
	DefaultMultiplier float64
	EMAAlpha          float64
	MinMultiplier     float64
	MaxMultiplier     float64
}

type EdgeStateFetcher interface {
	FetchEdgeState(ctx context.Context, edge EdgeMetadata) (EdgeTrafficState, error)
}

func BuildTrafficSnapshot(ctx context.Context, metadata EdgeMetadataFile, fetcher EdgeStateFetcher, opts TrafficOptions) (route.TrafficSnapshot, error) {
	if fetcher == nil {
		return route.TrafficSnapshot{}, fmt.Errorf("edge state fetcher is nil")
	}
	if err := validateSnapshotOptions(opts); err != nil {
		return route.TrafficSnapshot{}, err
	}

	snap := route.TrafficSnapshot{
		UpdatedAt:         time.Now().UTC(),
		DefaultMultiplier: opts.DefaultMultiplier,
		EdgeMultipliers:   make(map[[2]int]float64),
	}

	for _, edge := range metadata.Edges {
		state, err := fetcher.FetchEdgeState(ctx, edge)
		if err != nil {
			return route.TrafficSnapshot{}, fmt.Errorf("fetch edge state %d -> %d: %w", edge.FromStop, edge.ToStop, err)
		}
		if !state.HasData {
			continue
		}
		multiplier, err := smoothedMultiplier(state, opts.EMAAlpha)
		if err != nil {
			return route.TrafficSnapshot{}, fmt.Errorf("edge state %d -> %d: %w", edge.FromStop, edge.ToStop, err)
		}
		snap.EdgeMultipliers[[2]int{edge.FromStop, edge.ToStop}] = multiplier
	}

	return snap, nil
}

func ApplyEdgeTraffic(ctx context.Context, baseline [][]float64, metadata EdgeMetadataFile, fetcher EdgeStateFetcher, opts TrafficOptions) ([][]float64, route.TrafficSnapshot, error) {
	if err := validateApplyOptions(opts); err != nil {
		return nil, route.TrafficSnapshot{}, err
	}
	snap, err := BuildTrafficSnapshot(ctx, metadata, fetcher, opts)
	if err != nil {
		return nil, route.TrafficSnapshot{}, err
	}
	return route.ApplyTraffic(baseline, snap, opts.MinMultiplier, opts.MaxMultiplier), snap, nil
}

func validateSnapshotOptions(opts TrafficOptions) error {
	if opts.DefaultMultiplier <= 0 {
		return fmt.Errorf("default multiplier must be > 0, got %v", opts.DefaultMultiplier)
	}
	if opts.EMAAlpha <= 0 || opts.EMAAlpha > 1 {
		return fmt.Errorf("ema alpha must be > 0 and <= 1, got %v", opts.EMAAlpha)
	}
	return nil
}

func validateApplyOptions(opts TrafficOptions) error {
	if err := validateSnapshotOptions(opts); err != nil {
		return err
	}
	if opts.MinMultiplier <= 0 {
		return fmt.Errorf("min multiplier must be > 0, got %v", opts.MinMultiplier)
	}
	if opts.MaxMultiplier < opts.MinMultiplier {
		return fmt.Errorf("max multiplier must be >= min multiplier, got max %v min %v", opts.MaxMultiplier, opts.MinMultiplier)
	}
	return nil
}

func smoothedMultiplier(state EdgeTrafficState, alpha float64) (float64, error) {
	if state.CurrentMultiplier <= 0 {
		return 0, fmt.Errorf("current multiplier must be > 0, got %v", state.CurrentMultiplier)
	}
	if !state.HasPrevious {
		return state.CurrentMultiplier, nil
	}
	if state.PreviousMultiplier <= 0 {
		return 0, fmt.Errorf("previous multiplier must be > 0, got %v", state.PreviousMultiplier)
	}
	return alpha*state.CurrentMultiplier + (1-alpha)*state.PreviousMultiplier, nil
}
