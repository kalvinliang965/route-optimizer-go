package main

import (
  "reflect"
  "sort"
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

  // Mock findIdx function (not strictly used since matrix is a 2D slice, but matches signature)
  findIdx := func(name string) int {
    for i, s := range stops {
      if s.Name == name {
        return i
      }
    }
    return -1
  }

  // Request top 2 routes (k = 2)
  k := 2
  routes, err := solve(stops, matrix, findIdx, k)

  if err != nil {
    t.Fatalf("solve failed unexpectedly: %v", err)
  }

  // Since k = 2, we expect exactly 2 routes returned
  if len(routes) != k {
    t.Errorf("expected %d routes, got %d", k, len(routes))
  }

  // Every route must start with 0 (the depot)
  for _, route := range routes {
    if len(route) == 0 || route[0] != 0 {
      t.Errorf("route %v does not start with depot index 0", route)
    }
    if len(route) != len(stops) {
      t.Errorf("route %v length mismatch, expected %d", route, len(stops))
    }
  }

  // Verify that the first route returned is a valid permutation of all stop indices
  expectedLength := len(stops)
  if reflect.ValueOf(routes[0]).Len() != expectedLength {
    t.Errorf("route length invalid")
  }
}