package storage

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "artifact.json")
	want := []string{"a", "b"}
	if err := WriteJSON(path, want); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got []string
	if err := ReadJSON(path, &got); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
