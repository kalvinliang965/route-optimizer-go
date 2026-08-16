package traffic

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	DefaultMatchMaxDistanceM        = 80.0
	DefaultMatchMaxAverageDistanceM = 35.0
	DefaultMatchMinLinkPoints       = 2

	earthRadiusM = 6371008.8
)

type MatchOptions struct {
	MaxDistanceM        float64
	MaxAverageDistanceM float64
	MinLinkPoints       int
	MaxLinksPerEdge     int
	PreserveExisting    bool
}

type MatchSummary struct {
	EdgeCount          int
	MatchedEdgeCount   int
	TotalMatchedLinks  int
	CandidateLinkCount int
	SkippedLinkCount   int
}

type dotLinkGeometry struct {
	LinkID string
	Points []Coordinate
}

type dotLinkMatch struct {
	LinkID           string
	AverageDistanceM float64
	MaxDistanceM     float64
}

func MatchDOTLinks(metadata EdgeMetadataFile, records []DOTTrafficRecord, opts MatchOptions) (EdgeMetadataFile, MatchSummary, error) {
	opts = normalizeMatchOptions(opts)

	links, skipped := buildDOTLinkGeometries(records, opts.MinLinkPoints)
	out := copyEdgeMetadataFile(metadata)
	summary := MatchSummary{
		EdgeCount:          len(out.Edges),
		CandidateLinkCount: len(links),
		SkippedLinkCount:   skipped,
	}

	for i, edge := range out.Edges {
		candidates := matchEdgeToDOTLinks(edge, links, opts)

		matchedIDs := make([]string, 0, len(edge.MatchedDOTLinkIDs)+len(candidates))
		seen := make(map[string]bool, len(edge.MatchedDOTLinkIDs)+len(candidates))
		if opts.PreserveExisting {
			for _, linkID := range edge.MatchedDOTLinkIDs {
				linkID = strings.TrimSpace(linkID)
				if linkID == "" || seen[linkID] {
					continue
				}
				seen[linkID] = true
				matchedIDs = append(matchedIDs, linkID)
			}
		}

		for _, candidate := range candidates {
			if seen[candidate.LinkID] {
				continue
			}
			seen[candidate.LinkID] = true
			matchedIDs = append(matchedIDs, candidate.LinkID)
		}

		out.Edges[i].MatchedDOTLinkIDs = matchedIDs
		if len(matchedIDs) > 0 {
			summary.MatchedEdgeCount++
			summary.TotalMatchedLinks += len(matchedIDs)
		}
	}

	return out, summary, nil
}

func parseDOTLinkPoints(value string) ([]Coordinate, error) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return nil, fmt.Errorf("dot link_points is empty")
	}

	points := make([]Coordinate, 0, len(fields))
	for i, field := range fields {
		parts := strings.Split(field, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("dot link_points coordinate %d has %d values, want 2", i, len(parts))
		}
		lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return nil, fmt.Errorf("dot link_points coordinate %d latitude: %w", i, err)
		}
		lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("dot link_points coordinate %d longitude: %w", i, err)
		}
		points = append(points, Coordinate{Lat: lat, Lon: lon})
	}

	return points, nil
}

func normalizeMatchOptions(opts MatchOptions) MatchOptions {
	if opts.MaxDistanceM <= 0 {
		opts.MaxDistanceM = DefaultMatchMaxDistanceM
	}
	if opts.MaxAverageDistanceM <= 0 {
		opts.MaxAverageDistanceM = DefaultMatchMaxAverageDistanceM
	}
	if opts.MinLinkPoints <= 0 {
		opts.MinLinkPoints = DefaultMatchMinLinkPoints
	}
	return opts
}

func buildDOTLinkGeometries(records []DOTTrafficRecord, minLinkPoints int) ([]dotLinkGeometry, int) {
	latest := latestDOTRecordsByLinkID(records)
	linkIDs := make([]string, 0, len(latest))
	for linkID := range latest {
		linkIDs = append(linkIDs, linkID)
	}
	sort.Strings(linkIDs)

	links := make([]dotLinkGeometry, 0, len(linkIDs))
	skipped := 0
	for _, linkID := range linkIDs {
		record := latest[linkID]
		points, err := parseDOTLinkPoints(record.LinkPoints)
		if err != nil || len(points) < minLinkPoints {
			skipped++
			continue
		}
		links = append(links, dotLinkGeometry{
			LinkID: linkID,
			Points: points,
		})
	}
	return links, skipped
}

func matchEdgeToDOTLinks(edge EdgeMetadata, links []dotLinkGeometry, opts MatchOptions) []dotLinkMatch {
	if len(edge.Geometry) < 2 {
		return nil
	}

	matches := make([]dotLinkMatch, 0)
	for _, link := range links {
		averageDistance, maxDistance := linkDistanceToEdgeM(link.Points, edge.Geometry)
		if maxDistance > opts.MaxDistanceM || averageDistance > opts.MaxAverageDistanceM {
			continue
		}
		matches = append(matches, dotLinkMatch{
			LinkID:           link.LinkID,
			AverageDistanceM: averageDistance,
			MaxDistanceM:     maxDistance,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].AverageDistanceM != matches[j].AverageDistanceM {
			return matches[i].AverageDistanceM < matches[j].AverageDistanceM
		}
		if matches[i].MaxDistanceM != matches[j].MaxDistanceM {
			return matches[i].MaxDistanceM < matches[j].MaxDistanceM
		}
		return matches[i].LinkID < matches[j].LinkID
	})
	if opts.MaxLinksPerEdge > 0 && len(matches) > opts.MaxLinksPerEdge {
		matches = matches[:opts.MaxLinksPerEdge]
	}
	return matches
}

func linkDistanceToEdgeM(linkPoints, edgeGeometry []Coordinate) (float64, float64) {
	var sum float64
	var maxDistance float64
	for _, point := range linkPoints {
		distance := pointToPolylineDistanceM(point, edgeGeometry)
		sum += distance
		if distance > maxDistance {
			maxDistance = distance
		}
	}
	return sum / float64(len(linkPoints)), maxDistance
}

func pointToPolylineDistanceM(point Coordinate, line []Coordinate) float64 {
	if len(line) == 0 {
		return math.Inf(1)
	}
	if len(line) == 1 {
		return distanceBetweenCoordinatesM(point, line[0])
	}

	minDistance := math.Inf(1)
	for i := 0; i < len(line)-1; i++ {
		distance := pointToSegmentDistanceM(point, line[i], line[i+1])
		if distance < minDistance {
			minDistance = distance
		}
	}
	return minDistance
}

func pointToSegmentDistanceM(point, start, end Coordinate) float64 {
	ax, ay := projectCoordinateRelativeTo(point, start)
	bx, by := projectCoordinateRelativeTo(point, end)

	dx := bx - ax
	dy := by - ay
	if dx == 0 && dy == 0 {
		return math.Hypot(ax, ay)
	}

	t := -(ax*dx + ay*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	nearestX := ax + t*dx
	nearestY := ay + t*dy
	return math.Hypot(nearestX, nearestY)
}

func projectCoordinateRelativeTo(origin, point Coordinate) (float64, float64) {
	latRad := degreesToRadians(origin.Lat)
	x := degreesToRadians(point.Lon-origin.Lon) * earthRadiusM * math.Cos(latRad)
	y := degreesToRadians(point.Lat-origin.Lat) * earthRadiusM
	return x, y
}

func distanceBetweenCoordinatesM(a, b Coordinate) float64 {
	lat1 := degreesToRadians(a.Lat)
	lat2 := degreesToRadians(b.Lat)
	dLat := degreesToRadians(b.Lat - a.Lat)
	dLon := degreesToRadians(b.Lon - a.Lon)

	sinDLat := math.Sin(dLat / 2)
	sinDLon := math.Sin(dLon / 2)
	h := sinDLat*sinDLat + math.Cos(lat1)*math.Cos(lat2)*sinDLon*sinDLon
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

func degreesToRadians(value float64) float64 {
	return value * math.Pi / 180
}

func copyEdgeMetadataFile(metadata EdgeMetadataFile) EdgeMetadataFile {
	out := metadata
	out.Edges = make([]EdgeMetadata, len(metadata.Edges))
	for i, edge := range metadata.Edges {
		out.Edges[i] = edge
		out.Edges[i].Geometry = append([]Coordinate(nil), edge.Geometry...)
		out.Edges[i].Steps = append([]StepMetadata(nil), edge.Steps...)
		out.Edges[i].MatchedDOTLinkIDs = append([]string(nil), edge.MatchedDOTLinkIDs...)
	}
	return out
}
