package route

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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
			got := SecondsToMinutes(tt.seconds)
			if got != tt.want {
				t.Errorf("SecondsToMinutes(%v) = %v; want %v", tt.seconds, got, tt.want)
			}
		})
	}
}

func TestWriteJSON_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "out.json")
	want := []Stop{
		{Name: "A", Lat: 40.1, Lon: -74.1},
		{Name: "B", Lat: 40.2, Lon: -74.2},
	}
	if err := WriteJSON(path, want); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got []Stop
	if err := ReadJSON(path, &got); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v; want %#v", got, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}
}
