package optimizer

import (
	"reflect"
	"strings"
	"testing"
)

func TestSolverReturnsRankedRoundTrips(t *testing.T) {
	stops := []Stop{{Name: "Depot"}, {Name: "A"}, {Name: "B"}}
	matrix := Matrix{
		{0, 1, 5},
		{4, 0, 1},
		{1, 5, 0},
	}

	routes, err := NewSolver(10, 10).Solve(stops, matrix, 2)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("route count = %d, want 2", len(routes))
	}
	if !reflect.DeepEqual(routes[0].Path, []int{0, 1, 2, 0}) || routes[0].DurationSeconds != 3 {
		t.Fatalf("first route = %#v", routes[0])
	}
	if !reflect.DeepEqual(routes[1].Path, []int{0, 2, 1, 0}) || routes[1].DurationSeconds != 14 {
		t.Fatalf("second route = %#v", routes[1])
	}
}

func TestSolverTopKIsVariable(t *testing.T) {
	stops := []Stop{{}, {}, {}, {}}
	matrix := Matrix{
		{0, 1, 2, 3},
		{1, 0, 4, 5},
		{2, 4, 0, 6},
		{3, 5, 6, 0},
	}
	for _, topK := range []int{1, 3, 6} {
		routes, err := NewSolver(10, 10).Solve(stops, matrix, topK)
		if err != nil {
			t.Fatalf("Solve(topK=%d): %v", topK, err)
		}
		if len(routes) != topK {
			t.Fatalf("Solve(topK=%d) returned %d routes", topK, len(routes))
		}
	}
}

func TestSolverValidatesRequest(t *testing.T) {
	tests := []struct {
		name   string
		stops  []Stop
		matrix Matrix
		topK   int
		want   string
	}{
		{name: "no stops", matrix: Matrix{}, topK: 1, want: "at least one stop"},
		{name: "bad top k", stops: []Stop{{}}, matrix: Matrix{{0}}, topK: 0, want: "top_k"},
		{name: "bad matrix", stops: []Stop{{}, {}}, matrix: Matrix{{0}}, topK: 1, want: "matrix"},
		{name: "negative cost", stops: []Stop{{}}, matrix: Matrix{{-1}}, topK: 1, want: "finite non-negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSolver(10, 10).Solve(tt.stops, tt.matrix, tt.topK)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestSolverSingleStopReturnsToDepot(t *testing.T) {
	routes, err := NewSolver(10, 10).Solve([]Stop{{Name: "Depot"}}, Matrix{{0}}, 5)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if len(routes) != 1 || !reflect.DeepEqual(routes[0].Path, []int{0, 0}) {
		t.Fatalf("routes = %#v", routes)
	}
}
