// package main

// import (
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"net/url"
// )

// func main() {
// 	baseURL := "https://data.cityofnewyork.us/resource/i4gi-tjb9.json"

// 	params := url.Values{}
// 	// 700 east 16 street to times square manhattan community board 5
// 	params.Add("$where", "borough='Manhattan', link_points:'-73.974688, 40.729661, -73.9859724, 40.7570095'")
// 	params.Add("$order", "data_as_of DESC")
// 	params.Add("$limit", "5")

// 	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

// 	resp, err := http.Get(fullURL)
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer resp.Body.Close()

// 	var records []map[string]interface{}

// 	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
// 		panic(err)
// 	}

// 	for i, r := range records {
// 		fmt.Printf("[%d]: %v\n", i, r)
// 	}

// }
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	baseURL := "http://router.project-osrm.org"
	coords := "-73.974688,40.729661;-73.9859724,40.7570095"

	// Switched to the /route/ service to get detailed steps and path geometry
	apiURL := fmt.Sprintf("%s/route/v1/driving/%s?overview=full&steps=true&geometries=geojson", baseURL, coords)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("User-Agent", "pepsi-routing-app/1.0")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("osrm route request failed: %s", resp.Status))
	}

	var routeResp map[string]interface{}
	if err := json.Unmarshal(body, &routeResp); err != nil {
		panic(err)
	}

	fmt.Printf("OSRM route response: \n\n%v\n\n", routeResp)
}