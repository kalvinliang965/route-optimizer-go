package route

import (
	"testing"
	"time"
)

func TestApplyTraffic_DefaultMultiplier(t *testing.T) {
	baseline := [][]float64{
		{0, 100, 200},
		{110, 0, 300},
		{210, 310, 0},
	}
	snap := TrafficSnapshot{
		UpdatedAt:         time.Now(),
		DefaultMultiplier: 2.0,
		EdgeMultipliers:   map[[2]int]float64{},
	}

	got := ApplyTraffic(baseline, snap, 0.5, 5.0)

	want := [][]float64{
		{0, 200, 400},
		{220, 0, 600},
		{420, 620, 0},
	}
	assertMatrixEqual(t, got, want)
}

func TestApplyTraffic_EdgeOverride(t *testing.T) {
	baseline := [][]float64{
		{0, 100, 200},
		{110, 0, 300},
		{210, 310, 0},
	}
	snap := TrafficSnapshot{
		DefaultMultiplier: 1.0,
		EdgeMultipliers: map[[2]int]float64{
			{0, 1}: 1.5, // only this edge slowed
		},
	}

	got := ApplyTraffic(baseline, snap, 0.5, 5.0)

	if got[0][1] != 150 {
		t.Errorf("edge 0→1 = %v; want 150", got[0][1])
	}
	if got[0][2] != 200 {
		t.Errorf("edge 0→2 should use default 1.0: got %v", got[0][2])
	}
	if got[1][2] != 300 {
		t.Errorf("edge 1→2 should use default 1.0: got %v", got[1][2])
	}
}

func TestApplyTraffic_Clamps(t *testing.T) {
	baseline := [][]float64{
		{0, 100},
		{100, 0},
	}
	snap := TrafficSnapshot{
		DefaultMultiplier: 1.0,
		EdgeMultipliers: map[[2]int]float64{
			{0, 1}: 10.0, // above max
			{1, 0}: 0.1,  // below min
		},
	}

	got := ApplyTraffic(baseline, snap, 0.5, 5.0)

	if got[0][1] != 500 { // 100 * 5.0
		t.Errorf("max clamp: got %v; want 500", got[0][1])
	}
	if got[1][0] != 50 { // 100 * 0.5
		t.Errorf("min clamp: got %v; want 50", got[1][0])
	}
}

func TestApplyTraffic_DoesNotMutateBaseline(t *testing.T) {
	baseline := [][]float64{
		{0, 100},
		{100, 0},
	}
	original := [][]float64{
		{0, 100},
		{100, 0},
	}
	snap := TrafficSnapshot{
		DefaultMultiplier: 3.0,
		EdgeMultipliers:   map[[2]int]float64{},
	}

	_ = ApplyTraffic(baseline, snap, 0.5, 5.0)

	assertMatrixEqual(t, baseline, original)
}

func assertMatrixEqual(t *testing.T, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("row %d cols: got %d want %d", i, len(got[i]), len(want[i]))
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Errorf("matrix[%d][%d] = %v; want %v", i, j, got[i][j], want[i][j])
			}
		}
	}
}
