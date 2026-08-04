package route

import (
  "fmt"
  "reflect"
  "sort"
  "strings"
  "testing"
)

func TestPermute(t *testing.T) {
  input := []int{1, 2}

  var got [][]int
  permute(input, 0, &got)

  want := [][]int{
    {1, 2},
    {2, 1},
  }

  sortPermutations(got)
  sortPermutations(want)

  if !reflect.DeepEqual(got, want) {
    t.Errorf("permute(%v) = %v; want %v", input, got, want)
  }
}

func sortPermutations(perms [][]int) {
  for _, p := range perms {
    sort.Ints(p)
  }
  sort.Slice(perms, func(i, j int) bool {
    for idx := range perms[i] {
      if perms[i][idx] != perms[j][idx] {
        return perms[i][idx] < perms[j][idx]
      }
    }
    return false
  })
}

func TestSolveTopK(t *testing.T) {

  stops := []Stop{
    {Name: "Depot"}, // Index 0
    {Name: "SiteA"}, // Index 1
    {Name: "SiteB"}, // Index 2
    {Name: "SiteC"}, // Index 3
  }

  // Define a mock distance/duration matrix: matrix[from][to]
  // 0: Depot, 1: SiteA, 2: SiteB, 3: SiteC
  matrix := [][]float64{
    {0.0, 2.0, 5.0, 10.0}, // From Depot to others
    {2.0, 0.0, 1.0, 4.0},  // From SiteA to others
    {5.0, 1.0, 0.0, 2.0},  // From SiteB to others
    {10.0, 4.0, 2.0, 0.0}, // From SiteC to others
  }

  // Request top 2 routes (k = 2)
  k := 2
  routes, err := Solve(stops, matrix, k)

  if err != nil {
    t.Fatalf("solve failed unexpectedly: %v", err)
  }

  // Since k = 2, we expect exactly 2 routes returned
  if len(routes) != k {
    t.Errorf("expected %d routes, got %d", k, len(routes))
  }

  // Every route must start with 0 (the depot)
  for _, route := range routes {
    if len(route.Path) == 0 || route.Path[0] != 0 {
      t.Errorf("route %v does not start with depot index 0", route.Path)
    }
    if len(route.Path) != len(stops) {
      t.Errorf("route %v length mismatch, expected %d", route.Path, len(stops))
    }
  }

  // Verify that the first route returned is a valid permutation of all stop indices
  expectedLength := len(stops)
  if len(routes[0].Path) != expectedLength {
    t.Errorf("route length invalid")
  }
}

// TestSolve_RejectsTooManyStops ensures we fail fast instead of enumerating (n-1)!.
func TestSolve_RejectsTooManyStops(t *testing.T) {
  old := MaxStops
  MaxStops = 3
  defer func() { MaxStops = old }()

  stops := make([]Stop, 4) // exceeds MaxStops
  for i := range stops {
    stops[i] = Stop{Name: fmt.Sprintf("S%d", i)}
  }
  matrix := make([][]float64, len(stops))
  for i := range matrix {
    matrix[i] = make([]float64, len(stops))
  }

  _, err := Solve(stops, matrix, 2)
  if err == nil {
    t.Fatal("expected error when stop count exceeds MaxStops, got nil")
  }
  if !strings.Contains(err.Error(), "max") && !strings.Contains(err.Error(), "too many") {
    t.Errorf("error should mention the stop limit, got: %v", err)
  }
}

// TestSolveTopKKeepsShortestRoutes catches the inverted bounded-heap bug:
// a min-heap that pops when Len() > k discards the shortest routes and keeps the longest.
func TestSolveTopKKeepsShortestRoutes(t *testing.T) {
  stops := []Stop{
    {Name: "Depot"},
    {Name: "A"},
    {Name: "B"},
    {Name: "C"},
  }

  // Cheap chain Depot -> A -> B -> C (duration 3).
  // All other permutations are much more expensive (201 or 300).
  matrix := [][]float64{
    {0, 1, 100, 100},
    {0, 0, 1, 100},
    {0, 100, 0, 1},
    {0, 100, 100, 0},
  }

  const k = 2
  routes, err := Solve(stops, matrix, k)
  if err != nil {
    t.Fatalf("solve failed: %v", err)
  }
  if len(routes) != k {
    t.Fatalf("expected %d routes, got %d", k, len(routes))
  }

  // Ranked ascending: best first.
  if routes[0].Duration > routes[1].Duration {
    t.Errorf("routes not sorted ascending: got %.0f then %.0f", routes[0].Duration, routes[1].Duration)
  }

  // Best path must be the unique optimum Depot -> A -> B -> C.
  wantBestPath := []int{0, 1, 2, 3}
  wantBestDuration := 3.0
  if routes[0].Duration != wantBestDuration {
    t.Errorf("best duration = %.0f; want %.0f (heap likely kept longest routes)", routes[0].Duration, wantBestDuration)
  }
  if !reflect.DeepEqual(routes[0].Path, wantBestPath) {
    t.Errorf("best path = %v; want %v", routes[0].Path, wantBestPath)
  }

  // Both returned durations must be among the two shortest possible totals: {3, 201}.
  // (Worst totals are 300 — those must not appear when k=2.)
  allowed := map[float64]bool{3: true, 201: true}
  for i, r := range routes {
    if !allowed[r.Duration] {
      t.Errorf("route[%d] duration %.0f is not in top-%d shortest set {3, 201}", i, r.Duration, k)
    }
  }
}
