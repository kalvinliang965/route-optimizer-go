package geocode

import (
	"context"
	"net/http"
	"net/http/httptest"
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
