package main

import "testing"

func TestSecondsToMinutes(t *testing.T) {
  tests := []struct {
    name    string
    seconds float64
    want    float64
  }{
    {name: "150 seconds", seconds: 150, want: 2.5},
    {name: "60 seconds", seconds: 60, want: 1.0},
    {name: "zero", seconds: 0, want: 0},
  }

  for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
      got := secondsToMinutes(tt.seconds)
      if got != tt.want {
        t.Errorf("secondsToMinutes(%v) = %v; want %v", tt.seconds, got, tt.want)
      }
    })
  }
}
