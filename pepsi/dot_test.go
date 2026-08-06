package pepsi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDOTClientFetchTrafficRecordsBuildsSocrataQuery(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.Header.Get("X-App-Token"); got != "test-token" {
			t.Fatalf("X-App-Token = %q, want test-token", got)
		}
		if got := r.Header.Get("User-Agent"); got != "test-agent" {
			t.Fatalf("User-Agent = %q, want test-agent", got)
		}
		if got := r.URL.Query().Get("$select"); got != "link_id,speed,travel_time,status,data_as_of,link_name,borough,link_points,encoded_poly_line" {
			t.Fatalf("$select = %q", got)
		}
		if got := r.URL.Query().Get("$where"); got != "link_id in('link-a','link-b')" {
			t.Fatalf("$where = %q, want sorted unique link ids", got)
		}
		if got := r.URL.Query().Get("$order"); got != "data_as_of DESC" {
			t.Fatalf("$order = %q, want data_as_of DESC", got)
		}
		if got := r.URL.Query().Get("$limit"); got != "4" {
			t.Fatalf("$limit = %q, want 4", got)
		}

		writeDOTRecords(t, w, []DOTTrafficRecord{
			{LinkID: "link-a", Speed: "12.5", DataAsOf: "2026-08-05T12:00:00"},
			{LinkID: "link-b", Speed: "20", DataAsOf: "2026-08-05T12:00:00"},
		})
	}))
	defer server.Close()

	client := NewDOTClient(server.URL)
	client.AppToken = "test-token"
	client.UserAgent = "test-agent"
	client.ChunkSize = 10
	client.LimitPerLink = 2

	records, err := client.FetchTrafficRecords(context.Background(), []string{"link-b", "link-a", "link-a", ""})
	if err != nil {
		t.Fatalf("FetchTrafficRecords: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1", requestCount)
	}
	if len(records) != 2 {
		t.Fatalf("records length = %d, want 2", len(records))
	}
	if records[0].LinkID != "link-a" || records[1].LinkID != "link-b" {
		t.Fatalf("records = %#v, want link-a then link-b", records)
	}
}

func TestDOTClientFetchAllTrafficRecordsPaginates(t *testing.T) {
	requestOffsets := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("$select"); got != dotTrafficSelectFields {
			t.Fatalf("$select = %q", got)
		}
		if got := r.URL.Query().Get("$order"); got != "link_id ASC,data_as_of DESC" {
			t.Fatalf("$order = %q", got)
		}
		if got := r.URL.Query().Get("$limit"); got != "2" {
			t.Fatalf("$limit = %q, want 2", got)
		}

		offset := r.URL.Query().Get("$offset")
		requestOffsets = append(requestOffsets, offset)
		switch offset {
		case "0":
			writeDOTRecords(t, w, []DOTTrafficRecord{
				{LinkID: "link-a", Speed: "12.5", DataAsOf: "2026-08-05T12:00:00"},
				{LinkID: "link-b", Speed: "20", DataAsOf: "2026-08-05T12:00:00"},
			})
		case "2":
			writeDOTRecords(t, w, []DOTTrafficRecord{
				{LinkID: "link-c", Speed: "25", DataAsOf: "2026-08-05T12:00:00"},
			})
		default:
			t.Fatalf("unexpected offset: %s", offset)
		}
	}))
	defer server.Close()

	client := NewDOTClient(server.URL)
	records, err := client.FetchAllTrafficRecords(context.Background(), DOTFetchAllOptions{Limit: 2})
	if err != nil {
		t.Fatalf("FetchAllTrafficRecords: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("records length = %d, want 3", len(records))
	}
	if len(requestOffsets) != 2 || requestOffsets[0] != "0" || requestOffsets[1] != "2" {
		t.Fatalf("request offsets = %#v, want 0 then 2", requestOffsets)
	}
}

func TestBuildDOTEdgeStateFetcherConvertsMatchedLinksToMultipliers(t *testing.T) {
	metadata := dotTestMetadata()
	var gotWhere string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWhere = r.URL.Query().Get("$where")
		writeDOTRecords(t, w, []DOTTrafficRecord{
			{LinkID: "dot-a", Speed: "5", DataAsOf: "2026-08-05T12:00:00"},
			{LinkID: "dot-a", Speed: "20", DataAsOf: "2026-08-05T12:05:00"},
			{LinkID: "dot-b", Speed: "10", DataAsOf: "2026-08-05T12:00:00"},
		})
	}))
	defer server.Close()

	client := NewDOTClient(server.URL)
	fetcher, err := BuildDOTEdgeStateFetcher(context.Background(), client, metadata, nil)
	if err != nil {
		t.Fatalf("BuildDOTEdgeStateFetcher: %v", err)
	}
	if gotWhere != "link_id in('dot-a','dot-b','dot-missing')" {
		t.Fatalf("$where = %q, want unique sorted matched links", gotWhere)
	}

	state, err := fetcher.FetchEdgeState(context.Background(), metadata.Edges[0])
	if err != nil {
		t.Fatalf("FetchEdgeState: %v", err)
	}
	if !state.HasData {
		t.Fatal("HasData = false, want true")
	}

	// Baseline is 60 MPH. Latest dot-a speed is 20 MPH, dot-b is 10 MPH, so
	// average observed speed is 15 MPH and multiplier is 4.
	if !nearlyEqual(state.CurrentMultiplier, 4.0) {
		t.Fatalf("CurrentMultiplier = %v, want 4.0", state.CurrentMultiplier)
	}
}

func TestDOTEdgeStateFetcherFeedsEMA(t *testing.T) {
	metadata := EdgeMetadataFile{
		Edges: []EdgeMetadata{
			{
				FromStop:            0,
				ToStop:              1,
				BaselineDurationSec: 60,
				BaselineDistanceM:   1609.344,
				MatchedDOTLinkIDs:   []string{"dot-a"},
			},
		},
	}
	fetcher := NewDOTEdgeStateFetcher(
		[]DOTTrafficRecord{{LinkID: "dot-a", Speed: "30", DataAsOf: "2026-08-05T12:00:00"}},
		map[[2]int]float64{{0, 1}: 1.2},
	)

	snap, err := BuildTrafficSnapshot(context.Background(), metadata, fetcher, testTrafficOptions())
	if err != nil {
		t.Fatalf("BuildTrafficSnapshot: %v", err)
	}

	// Baseline is 60 MPH, live speed is 30 MPH, current multiplier is 2.0.
	// alpha 0.3: 0.3*2.0 + 0.7*1.2 = 1.44.
	if got := snap.EdgeMultipliers[[2]int{0, 1}]; !nearlyEqual(got, 1.44) {
		t.Fatalf("EMA multiplier = %v, want 1.44", got)
	}
}

func TestDOTEdgeStateFetcherFallsBackWhenNoRecordMatches(t *testing.T) {
	edge := EdgeMetadata{
		FromStop:            0,
		ToStop:              1,
		BaselineDurationSec: 60,
		BaselineDistanceM:   1609.344,
		MatchedDOTLinkIDs:   []string{"dot-missing"},
	}
	fetcher := NewDOTEdgeStateFetcher(
		[]DOTTrafficRecord{{LinkID: "dot-a", Speed: "30", DataAsOf: "2026-08-05T12:00:00"}},
		nil,
	)

	state, err := fetcher.FetchEdgeState(context.Background(), edge)
	if err != nil {
		t.Fatalf("FetchEdgeState: %v", err)
	}
	if state.HasData {
		t.Fatal("HasData = true, want false")
	}
}

func dotTestMetadata() EdgeMetadataFile {
	return EdgeMetadataFile{
		Source: EdgeMetadataSource,
		Edges: []EdgeMetadata{
			{
				FromStop:            0,
				ToStop:              1,
				BaselineDurationSec: 60,
				BaselineDistanceM:   1609.344,
				MatchedDOTLinkIDs:   []string{"dot-b", "dot-a", "dot-missing"},
			},
			{
				FromStop:            1,
				ToStop:              0,
				BaselineDurationSec: 120,
				BaselineDistanceM:   1609.344,
				MatchedDOTLinkIDs:   []string{"dot-a"},
			},
		},
	}
}

func writeDOTRecords(t *testing.T, w http.ResponseWriter, records []DOTTrafficRecord) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(records); err != nil {
		t.Fatalf("encode records: %v", err)
	}
}
