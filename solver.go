package main

import (
  "container/heap"
  "fmt"
)

type RouteItem struct {
  Path     []int
  Duration float64
}

type RouteHeap []RouteItem

func (h RouteHeap) Len() int           { return len(h) }
func (h RouteHeap) Less(i, j int) bool { return h[i].Duration < h[j].Duration }
func (h RouteHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *RouteHeap) Push(x interface{}) {
  *h = append(*h, x.(RouteItem))
}

func (h *RouteHeap) Pop() interface{} {
  old := *h
  n := len(old)
  item := old[n-1]
  *h = old[0 : n-1]
  return item
}

func permute(arr []int, start int, result *[][]int) {
  if start == len(arr) {
    temp := make([]int, len(arr))
    copy(temp, arr)
    *result = append(*result, temp)
    return
  }
  for i := start; i < len(arr); i++ {
    arr[i], arr[start] = arr[start], arr[i]
    permute(arr, start+1, result)
    arr[start], arr[i] = arr[i], arr[start]
  }
}

// we assume that stops < 15
func solve(stops []Stop, matrix [][]float64, findIdx func(string) int, k int) ([][]int, error) {
  n := len(stops)
  if n <= 1 {
    return [][]int{{0}}, nil
  }

  // indices of stops (skip index 0 since it's the fixed start/depot)
  indices := make([]int, n-1)
  for i := 0; i < n-1; i++ {
    indices[i] = i + 1
  }

  // permutate the remaining route indices
  var results [][]int
  permute(indices, 0, &results)

  h := &RouteHeap{}
  heap.Init(h)

  for i := 0; i < len(results); i++ {
    // Full route starts with index 0 (the depot)
    fullRoute := append([]int{0}, results[i]...)
    duration := 0.0

    for j := 0; j < len(fullRoute)-1; j++ {
      fromIdx := fullRoute[j]
      toIdx := fullRoute[j+1]

      // Since matrix is a 2D slice [][]float64, we can index directly using int indices
      if fromIdx < len(matrix) && toIdx < len(matrix[fromIdx]) {
        duration += matrix[fromIdx][toIdx]
      } else {
        return nil, fmt.Errorf("failed to solve: matrix indices out of bounds")
      }
    }

    heap.Push(h, RouteItem{
      Path:     fullRoute,
      Duration: duration,
    })

    // For a min-heap, popping when Len > k removes the longest durations,
    // leaving us with the top k shortest routes.
    if h.Len() > k {
      heap.Pop(h)
    }
  }

  // Extract results (pop from min-heap, which gives them in ascending order of duration)
  var res [][]int
  // Temporary slice to reverse or collect elements since heap pop gives shortest first
  tempRes := make([]RouteItem, h.Len())
  for i := len(tempRes) - 1; i >= 0; i-- {
    tempRes[i] = heap.Pop(h).(RouteItem)
  }

  for _, item := range tempRes {
    res = append(res, item.Path)
  }

  return res, nil
}