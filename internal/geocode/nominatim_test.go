package geocode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNominatimGeocode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "Times Square" {
			t.Errorf("request URL = %s", r.URL.String())
		}
		if got := r.Header.Get("User-Agent"); got != "test-agent" {
			t.Errorf("User-Agent = %q", got)
		}
		_, _ = w.Write([]byte(`[{"display_name":"Times Square, New York","lat":"40.757","lon":"-73.986"}]`))
	}))
	defer server.Close()

	client := NewNominatim(server.URL, "test-agent", 0)
	stop, err := client.Geocode(context.Background(), "Times Square")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if stop.Name != "Times Square, New York" || stop.Lat != 40.757 || stop.Lon != -73.986 {
		t.Fatalf("stop = %#v", stop)
	}
}

func TestNominatimRejectsInvalidResult(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{name: "blank name", response: `[{"display_name":"","lat":"40","lon":"-73"}]`},
		{name: "non-finite latitude", response: `[{"display_name":"Invalid","lat":"NaN","lon":"-73"}]`},
		{name: "latitude out of range", response: `[{"display_name":"Invalid","lat":"91","lon":"-73"}]`},
		{name: "longitude out of range", response: `[{"display_name":"Invalid","lat":"40","lon":"-181"}]`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(test.response))
			}))
			defer server.Close()

			client := NewNominatim(server.URL, "test-agent", 0)
			if _, err := client.Geocode(context.Background(), "Invalid"); err == nil || !strings.Contains(err.Error(), "invalid Nominatim result") {
				t.Fatalf("Geocode error = %v", err)
			}
		})
	}
}
