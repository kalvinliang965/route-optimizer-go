package main

import "time"

// TrafficSnapshot holds congestion multipliers to apply onto an OSRM baseline matrix.
// This is element-wise scaling (Hadamard), not linear-algebra matrix multiplication.
type TrafficSnapshot struct {
	UpdatedAt         time.Time
	DefaultMultiplier float64
	EdgeMultipliers   map[[2]int]float64 // (fromIdx, toIdx) → factor
}

// ApplyTraffic returns a new duration matrix where each off-diagonal edge is
// baseline[i][j] * clamp(multiplier). The baseline matrix is never modified.
func ApplyTraffic(baseline [][]float64, snap TrafficSnapshot, minM, maxM float64) [][]float64 {
	n := len(baseline)
	out := make([][]float64, n)
	for i := range baseline {
		out[i] = make([]float64, len(baseline[i]))
		for j := range baseline[i] {
			if i == j {
				out[i][j] = 0
				continue
			}
			mult := snap.DefaultMultiplier
			if snap.EdgeMultipliers != nil {
				if m, ok := snap.EdgeMultipliers[[2]int{i, j}]; ok {
					mult = m
				}
			}
			out[i][j] = baseline[i][j] * clampMultiplier(mult, minM, maxM)
		}
	}
	return out
}

func clampMultiplier(m, minM, maxM float64) float64 {
	if m < minM {
		return minM
	}
	if m > maxM {
		return maxM
	}
	return m
}
