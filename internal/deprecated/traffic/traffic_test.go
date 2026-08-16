package traffic

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestBuildTrafficSnapshotFetchesEachEdgeState(t *testing.T) {
	metadata := testEdgeMetadataFile()
	fetcher := fakeEdgeStateFetcher{
		states: map[[2]int]EdgeTrafficState{
			{0, 1}: {HasData: true, CurrentMultiplier: 1.25},
			{1, 0}: {HasData: true, CurrentMultiplier: 1.50},
		},
	}

	snap, err := BuildTrafficSnapshot(context.Background(), metadata, &fetcher, testTrafficOptions())
	if err != nil {
		t.Fatalf("BuildTrafficSnapshot: %v", err)
	}

	if snap.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt is zero")
	}
	if snap.DefaultMultiplier != 1.0 {
		t.Fatalf("DefaultMultiplier = %v, want 1.0", snap.DefaultMultiplier)
	}
	if len(fetcher.calls) != 2 {
		t.Fatalf("fetch calls = %d, want 2", len(fetcher.calls))
	}
	if fetcher.calls[0] != [2]int{0, 1} || fetcher.calls[1] != [2]int{1, 0} {
		t.Fatalf("fetch calls = %#v, want [0 1], [1 0]", fetcher.calls)
	}
	if got := snap.EdgeMultipliers[[2]int{0, 1}]; got != 1.25 {
		t.Fatalf("0->1 multiplier = %v, want 1.25", got)
	}
	if got := snap.EdgeMultipliers[[2]int{1, 0}]; got != 1.50 {
		t.Fatalf("1->0 multiplier = %v, want 1.50", got)
	}
}

func TestBuildTrafficSnapshotSkipsEdgesWithoutState(t *testing.T) {
	metadata := testEdgeMetadataFile()
	fetcher := fakeEdgeStateFetcher{
		states: map[[2]int]EdgeTrafficState{
			{0, 1}: {HasData: true, CurrentMultiplier: 1.25},
			{1, 0}: {HasData: false},
		},
	}

	snap, err := BuildTrafficSnapshot(context.Background(), metadata, &fetcher, testTrafficOptions())
	if err != nil {
		t.Fatalf("BuildTrafficSnapshot: %v", err)
	}

	if len(snap.EdgeMultipliers) != 1 {
		t.Fatalf("edge multiplier count = %d, want 1", len(snap.EdgeMultipliers))
	}
	if _, ok := snap.EdgeMultipliers[[2]int{1, 0}]; ok {
		t.Fatal("1->0 multiplier is present, want default fallback")
	}
}

func TestBuildTrafficSnapshotPropagatesEdgeStateError(t *testing.T) {
	metadata := testEdgeMetadataFile()
	fetcher := fakeEdgeStateFetcher{
		errs: map[[2]int]error{
			{1, 0}: errors.New("dot unavailable"),
		},
	}

	_, err := BuildTrafficSnapshot(context.Background(), metadata, &fetcher, testTrafficOptions())
	if err == nil {
		t.Fatal("BuildTrafficSnapshot error = nil, want fetch error")
	}
	if !strings.Contains(err.Error(), "fetch edge state 1 -> 0") {
		t.Fatalf("error = %q, want edge context", err)
	}
}

func TestBuildTrafficSnapshotAppliesEMA(t *testing.T) {
	metadata := testEdgeMetadataFile()
	fetcher := fakeEdgeStateFetcher{
		states: map[[2]int]EdgeTrafficState{
			{0, 1}: {
				HasData:            true,
				CurrentMultiplier:  1.8,
				PreviousMultiplier: 1.2,
				HasPrevious:        true,
			},
		},
	}

	snap, err := BuildTrafficSnapshot(context.Background(), metadata, &fetcher, testTrafficOptions())
	if err != nil {
		t.Fatalf("BuildTrafficSnapshot: %v", err)
	}

	// alpha 0.3: 0.3*1.8 + 0.7*1.2 = 1.38
	if got := snap.EdgeMultipliers[[2]int{0, 1}]; !nearlyEqual(got, 1.38) {
		t.Fatalf("0->1 EMA multiplier = %v, want 1.38", got)
	}
}

func TestApplyEdgeTrafficAppliesSnapshotToBaseline(t *testing.T) {
	metadata := testEdgeMetadataFile()
	fetcher := fakeEdgeStateFetcher{
		states: map[[2]int]EdgeTrafficState{
			{0, 1}: {HasData: true, CurrentMultiplier: 1.5},
			{1, 0}: {HasData: false},
		},
	}
	baseline := [][]float64{
		{0, 100},
		{200, 0},
	}

	adjusted, snap, err := ApplyEdgeTraffic(context.Background(), baseline, metadata, &fetcher, testTrafficOptions())
	if err != nil {
		t.Fatalf("ApplyEdgeTraffic: %v", err)
	}

	if got := snap.EdgeMultipliers[[2]int{0, 1}]; got != 1.5 {
		t.Fatalf("snapshot 0->1 multiplier = %v, want 1.5", got)
	}
	if adjusted[0][1] != 150 {
		t.Fatalf("adjusted 0->1 = %v, want 150", adjusted[0][1])
	}
	if adjusted[1][0] != 200 {
		t.Fatalf("adjusted 1->0 = %v, want default baseline 200", adjusted[1][0])
	}
	if baseline[0][1] != 100 {
		t.Fatalf("baseline mutated: baseline 0->1 = %v, want 100", baseline[0][1])
	}
}

func TestFixtureEdgeStateFetcherFullFlow(t *testing.T) {
	metadata := testEdgeMetadataFile()
	fetcher, err := LoadFixtureEdgeStateFetcher("testdata/edge_state_fixture.json")
	if err != nil {
		t.Fatalf("LoadFixtureEdgeStateFetcher: %v", err)
	}
	baseline := [][]float64{
		{0, 100},
		{200, 0},
	}

	adjusted, snap, err := ApplyEdgeTraffic(context.Background(), baseline, metadata, fetcher, testTrafficOptions())
	if err != nil {
		t.Fatalf("ApplyEdgeTraffic: %v", err)
	}

	if got := snap.EdgeMultipliers[[2]int{0, 1}]; !nearlyEqual(got, 1.38) {
		t.Fatalf("fixture 0->1 EMA multiplier = %v, want 1.38", got)
	}
	if got := snap.EdgeMultipliers[[2]int{1, 0}]; !nearlyEqual(got, 1.1) {
		t.Fatalf("fixture 1->0 multiplier = %v, want 1.1", got)
	}
	if !nearlyEqual(adjusted[0][1], 138) {
		t.Fatalf("adjusted 0->1 = %v, want 138", adjusted[0][1])
	}
	if !nearlyEqual(adjusted[1][0], 220) {
		t.Fatalf("adjusted 1->0 = %v, want 220", adjusted[1][0])
	}
}

func testEdgeMetadataFile() EdgeMetadataFile {
	return EdgeMetadataFile{
		Source: EdgeMetadataSource,
		Edges: []EdgeMetadata{
			{FromStop: 0, ToStop: 1, MatchedDOTLinkIDs: []string{"dot-a"}},
			{FromStop: 1, ToStop: 0, MatchedDOTLinkIDs: []string{"dot-b"}},
		},
	}
}

func testTrafficOptions() TrafficOptions {
	return TrafficOptions{
		DefaultMultiplier: 1.0,
		EMAAlpha:          0.3,
		MinMultiplier:     0.5,
		MaxMultiplier:     2.0,
	}
}

type fakeEdgeStateFetcher struct {
	states map[[2]int]EdgeTrafficState
	errs   map[[2]int]error
	calls  [][2]int
}

func (f *fakeEdgeStateFetcher) FetchEdgeState(ctx context.Context, edge EdgeMetadata) (EdgeTrafficState, error) {
	key := [2]int{edge.FromStop, edge.ToStop}
	f.calls = append(f.calls, key)
	if err := f.errs[key]; err != nil {
		return EdgeTrafficState{}, err
	}
	return f.states[key], nil
}

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0000001
}
