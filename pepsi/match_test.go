package pepsi

import "testing"

func TestParseDOTLinkPoints(t *testing.T) {
	points, err := parseDOTLinkPoints("40.000000,-73.000000 40.001000,-73.001000")
	if err != nil {
		t.Fatalf("parseDOTLinkPoints: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points length = %d, want 2", len(points))
	}
	if points[0].Lat != 40.0 || points[0].Lon != -73.0 {
		t.Fatalf("first point = %#v, want 40,-73", points[0])
	}
	if points[1].Lat != 40.001 || points[1].Lon != -73.001 {
		t.Fatalf("second point = %#v, want 40.001,-73.001", points[1])
	}
}

func TestMatchDOTLinksAttachesNearbyLinks(t *testing.T) {
	metadata := EdgeMetadataFile{
		Source: EdgeMetadataSource,
		Edges: []EdgeMetadata{
			{
				FromStop: 0,
				ToStop:   1,
				Geometry: []Coordinate{
					{Lat: 40.0000, Lon: -73.0000},
					{Lat: 40.0005, Lon: -73.0005},
					{Lat: 40.0010, Lon: -73.0010},
				},
			},
			{
				FromStop: 1,
				ToStop:   0,
				Geometry: []Coordinate{
					{Lat: 40.1000, Lon: -73.1000},
					{Lat: 40.1010, Lon: -73.1010},
				},
			},
		},
	}
	records := []DOTTrafficRecord{
		{
			LinkID:     "near-link",
			LinkPoints: "40.000100,-73.000100 40.000900,-73.000900",
			DataAsOf:   "2026-08-05T12:00:00",
		},
		{
			LinkID:     "far-link",
			LinkPoints: "40.010000,-73.010000 40.011000,-73.011000",
			DataAsOf:   "2026-08-05T12:00:00",
		},
	}

	matched, summary, err := MatchDOTLinks(metadata, records, MatchOptions{
		MaxDistanceM:        25,
		MaxAverageDistanceM: 15,
	})
	if err != nil {
		t.Fatalf("MatchDOTLinks: %v", err)
	}

	if summary.EdgeCount != 2 {
		t.Fatalf("summary.EdgeCount = %d, want 2", summary.EdgeCount)
	}
	if summary.CandidateLinkCount != 2 {
		t.Fatalf("summary.CandidateLinkCount = %d, want 2", summary.CandidateLinkCount)
	}
	if summary.MatchedEdgeCount != 1 {
		t.Fatalf("summary.MatchedEdgeCount = %d, want 1", summary.MatchedEdgeCount)
	}
	if summary.TotalMatchedLinks != 1 {
		t.Fatalf("summary.TotalMatchedLinks = %d, want 1", summary.TotalMatchedLinks)
	}
	if got := matched.Edges[0].MatchedDOTLinkIDs; len(got) != 1 || got[0] != "near-link" {
		t.Fatalf("edge 0 matched IDs = %#v, want near-link", got)
	}
	if got := matched.Edges[1].MatchedDOTLinkIDs; len(got) != 0 {
		t.Fatalf("edge 1 matched IDs = %#v, want empty", got)
	}
	if len(metadata.Edges[0].MatchedDOTLinkIDs) != 0 {
		t.Fatalf("original metadata mutated: %#v", metadata.Edges[0].MatchedDOTLinkIDs)
	}
}

func TestMatchDOTLinksPreservesExistingMatches(t *testing.T) {
	metadata := EdgeMetadataFile{
		Edges: []EdgeMetadata{
			{
				FromStop:          0,
				ToStop:            1,
				MatchedDOTLinkIDs: []string{"manual-link"},
				Geometry: []Coordinate{
					{Lat: 40.0000, Lon: -73.0000},
					{Lat: 40.0010, Lon: -73.0010},
				},
			},
		},
	}
	records := []DOTTrafficRecord{
		{LinkID: "near-link", LinkPoints: "40.000100,-73.000100 40.000900,-73.000900"},
	}

	matched, _, err := MatchDOTLinks(metadata, records, MatchOptions{
		MaxDistanceM:        25,
		MaxAverageDistanceM: 15,
		PreserveExisting:    true,
	})
	if err != nil {
		t.Fatalf("MatchDOTLinks: %v", err)
	}

	got := matched.Edges[0].MatchedDOTLinkIDs
	if len(got) != 2 || got[0] != "manual-link" || got[1] != "near-link" {
		t.Fatalf("matched IDs = %#v, want manual-link then near-link", got)
	}
}

func TestMatchDOTLinksUsesLatestRecordPerLink(t *testing.T) {
	metadata := EdgeMetadataFile{
		Edges: []EdgeMetadata{
			{
				FromStop: 0,
				ToStop:   1,
				Geometry: []Coordinate{
					{Lat: 40.0000, Lon: -73.0000},
					{Lat: 40.0010, Lon: -73.0010},
				},
			},
		},
	}
	records := []DOTTrafficRecord{
		{
			LinkID:     "same-link",
			LinkPoints: "40.010000,-73.010000 40.011000,-73.011000",
			DataAsOf:   "2026-08-05T12:00:00",
		},
		{
			LinkID:     "same-link",
			LinkPoints: "40.000100,-73.000100 40.000900,-73.000900",
			DataAsOf:   "2026-08-05T12:05:00",
		},
	}

	matched, _, err := MatchDOTLinks(metadata, records, MatchOptions{
		MaxDistanceM:        25,
		MaxAverageDistanceM: 15,
	})
	if err != nil {
		t.Fatalf("MatchDOTLinks: %v", err)
	}

	if got := matched.Edges[0].MatchedDOTLinkIDs; len(got) != 1 || got[0] != "same-link" {
		t.Fatalf("matched IDs = %#v, want same-link from latest record", got)
	}
}
