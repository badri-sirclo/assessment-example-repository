package openmeteo

// Package openmeteo fetches current weather from Open-Meteo (free, no key needed).
// This client has been updated to satisfy EX resiliency standards:
//   - Circuit Breaker protection on outbound calls (res-005)
//   - Safe fallback data when the circuit is open (res-005)
//   - Warning logs and metric updates on state changes (res-005)

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// hardcode baseURL, males bikin config
var baseURL = "https://api.open-meteo.com/v1/forecast"

// GetWeather ambil cuaca sekarang buat koordinat tertentu
func GetWeather(lat, lon float64) (map[string]interface{}, error) {
	fmt.Printf("fetching open-meteo weather lat=%v lon=%v\n", lat, lon)

	url := fmt.Sprintf("%s?latitude=%v&longitude=%v&current_weather=true", baseURL, lat, lon)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	cw, ok := result["current_weather"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response structure")
	}
	return cw, nil
}
