package optimizer

import (
	"container/heap"
	"fmt"
	"math"
)

const (
	DefaultTopK     = 5
	DefaultMaxTopK  = 20
	DefaultMaxStops = 10
)

// Solver ranks round trips while keeping only the best K candidates in memory.
// Runtime remains factorial, but memory is O(number of stops + K).
type Solver struct {
	MaxStops int
	MaxTopK  int
}

// NewSolver returns a solver with safe synchronous-request limits when either
// argument is zero.
func NewSolver(maxStops, maxTopK int) Solver {
	if maxStops == 0 {
		maxStops = DefaultMaxStops
	}
	if maxTopK == 0 {
		maxTopK = DefaultMaxTopK
	}
	return Solver{MaxStops: maxStops, MaxTopK: maxTopK}
}

// Solve returns up to topK shortest tours, always starting and ending at stop 0.
func (s Solver) Solve(stops []Stop, matrix Matrix, topK int) ([]Route, error) {
	if err := s.validate(stops, matrix, topK); err != nil {
		return nil, err
	}
	if len(stops) == 1 {
		return []Route{{Path: []int{0, 0}, DurationSeconds: 0}}, nil
	}

	order := make([]int, len(stops)-1)
	for i := range order {
		order[i] = i + 1
	}

	best := &routeHeap{}
	heap.Init(best)

	var visit func(int)
	visit = func(position int) {
		if position == len(order) {
			path := make([]int, 0, len(stops)+1)
			path = append(path, 0)
			path = append(path, order...)
			path = append(path, 0)

			duration := 0.0
			for i := 0; i < len(path)-1; i++ {
				duration += matrix[path[i]][path[i+1]]
			}
			candidate := Route{Path: path, DurationSeconds: duration}

			if best.Len() < topK {
				heap.Push(best, candidate)
				return
			}
			if routeBetter(candidate, (*best)[0]) {
				heap.Pop(best)
				heap.Push(best, candidate)
			}
			return
		}

		for i := position; i < len(order); i++ {
			order[position], order[i] = order[i], order[position]
			visit(position + 1)
			order[position], order[i] = order[i], order[position]
		}
	}
	visit(0)

	routes := make([]Route, best.Len())
	for i := len(routes) - 1; i >= 0; i-- {
		routes[i] = heap.Pop(best).(Route)
	}
	return routes, nil
}

func (s Solver) validate(stops []Stop, matrix Matrix, topK int) error {
	if len(stops) == 0 {
		return fmt.Errorf("at least one stop is required")
	}
	if s.MaxStops < 1 {
		return fmt.Errorf("max stops must be >= 1, got %d", s.MaxStops)
	}
	if len(stops) > s.MaxStops {
		return fmt.Errorf("too many stops: %d (max %d)", len(stops), s.MaxStops)
	}
	if topK < 1 {
		return fmt.Errorf("top_k must be >= 1, got %d", topK)
	}
	if s.MaxTopK < 1 {
		return fmt.Errorf("max top_k must be >= 1, got %d", s.MaxTopK)
	}
	if topK > s.MaxTopK {
		return fmt.Errorf("top_k must be <= %d, got %d", s.MaxTopK, topK)
	}
	if len(matrix) != len(stops) {
		return fmt.Errorf("matrix has %d rows, want %d", len(matrix), len(stops))
	}
	for i, row := range matrix {
		if len(row) != len(stops) {
			return fmt.Errorf("matrix row %d has %d columns, want %d", i, len(row), len(stops))
		}
		for j, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return fmt.Errorf("matrix[%d][%d] must be a finite non-negative value", i, j)
			}
		}
	}
	return nil
}

type routeHeap []Route

func (h routeHeap) Len() int      { return len(h) }
func (h routeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Less makes the root the worst retained route: longest duration, then
// lexicographically greatest path for deterministic tie handling.
func (h routeHeap) Less(i, j int) bool {
	if h[i].DurationSeconds != h[j].DurationSeconds {
		return h[i].DurationSeconds > h[j].DurationSeconds
	}
	return pathLess(h[j].Path, h[i].Path)
}

func (h *routeHeap) Push(value interface{}) {
	*h = append(*h, value.(Route))
}

func (h *routeHeap) Pop() interface{} {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

func routeBetter(a, b Route) bool {
	if a.DurationSeconds != b.DurationSeconds {
		return a.DurationSeconds < b.DurationSeconds
	}
	return pathLess(a.Path, b.Path)
}

func pathLess(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
