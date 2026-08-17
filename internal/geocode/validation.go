package geocode

import (
	"fmt"
	"math"
	"strings"

	"route-optimizer-go/internal/optimizer"
)

func validateResolvedStop(stop optimizer.Stop) error {
	if strings.TrimSpace(stop.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if math.IsNaN(stop.Lat) || math.IsInf(stop.Lat, 0) || stop.Lat < -90 || stop.Lat > 90 {
		return fmt.Errorf("latitude must be finite and between -90 and 90")
	}
	if math.IsNaN(stop.Lon) || math.IsInf(stop.Lon, 0) || stop.Lon < -180 || stop.Lon > 180 {
		return fmt.Errorf("longitude must be finite and between -180 and 180")
	}
	return nil
}
