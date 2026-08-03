package main

type OSMTripResponse struct {
  Code      string     `json:"code"`
  Waypoints []Waypoint `json:"waypoints"`
  Trips     []Trip     `json:"trips"`
}

type Waypoint struct {
  Name          string    `json:"name"`
  Location      []float64 `json:"Location"`
  WaypointIndex int       `json:"waypoint_index"`
  TripsIndex    int       `json:"trips_index"`
}

type Trip struct {
  Geometry string  `json:"geometry"`
  Duration float64 `json:"duration"`
  Distance float64 `json:"distance"`
}

type AddressStruct struct {
  Name string `json:"display_name"`
  Lon  string `json:"lon"`
  Lat  string `json:"lat"`
}

type Stop struct {
  Name string
  Lon  float64
  Lat  float64
}