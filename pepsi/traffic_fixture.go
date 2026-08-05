package pepsi

import (
	"context"
	"fmt"

	"route-optimizer-go/internal/route"
)

type FixtureEdgeStateFetcher struct {
	states map[[2]int]EdgeTrafficState
}

type edgeStateFixtureFile struct {
	Edges []edgeStateFixtureEntry `json:"edges"`
}

type edgeStateFixtureEntry struct {
	FromStop           int     `json:"from_stop"`
	ToStop             int     `json:"to_stop"`
	CurrentMultiplier  float64 `json:"current_multiplier"`
	PreviousMultiplier float64 `json:"previous_multiplier,omitempty"`
}

func LoadFixtureEdgeStateFetcher(path string) (FixtureEdgeStateFetcher, error) {
	var file edgeStateFixtureFile
	if err := route.ReadJSON(path, &file); err != nil {
		return FixtureEdgeStateFetcher{}, fmt.Errorf("read edge state fixture %s: %w", path, err)
	}

	states := make(map[[2]int]EdgeTrafficState, len(file.Edges))
	for i, edge := range file.Edges {
		if edge.FromStop < 0 || edge.ToStop < 0 {
			return FixtureEdgeStateFetcher{}, fmt.Errorf("edge state fixture %s edges[%d] has negative stop index", path, i)
		}
		if edge.FromStop == edge.ToStop {
			return FixtureEdgeStateFetcher{}, fmt.Errorf("edge state fixture %s edges[%d] from_stop == to_stop", path, i)
		}
		if edge.CurrentMultiplier <= 0 {
			return FixtureEdgeStateFetcher{}, fmt.Errorf("edge state fixture %s edges[%d] current_multiplier must be > 0", path, i)
		}
		if edge.PreviousMultiplier < 0 {
			return FixtureEdgeStateFetcher{}, fmt.Errorf("edge state fixture %s edges[%d] previous_multiplier must be >= 0", path, i)
		}
		states[[2]int{edge.FromStop, edge.ToStop}] = EdgeTrafficState{
			HasData:            true,
			CurrentMultiplier:  edge.CurrentMultiplier,
			PreviousMultiplier: edge.PreviousMultiplier,
			HasPrevious:        edge.PreviousMultiplier > 0,
		}
	}

	return FixtureEdgeStateFetcher{states: states}, nil
}

func (f FixtureEdgeStateFetcher) FetchEdgeState(ctx context.Context, edge EdgeMetadata) (EdgeTrafficState, error) {
	state, ok := f.states[[2]int{edge.FromStop, edge.ToStop}]
	if !ok {
		return EdgeTrafficState{HasData: false}, nil
	}
	return state, nil
}
