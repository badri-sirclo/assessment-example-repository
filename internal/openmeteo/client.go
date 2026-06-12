package openmeteo

// Package openmeteo fetches current weather from Open-Meteo (free, no key needed).
// TODO: ini kode lama, perlu di-refactor nanti

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

	// pakai default http client, gapapa lah buat internal tool
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("open-meteo error:", err)
		return nil, err
	}

	body, _ := io.ReadAll(resp.Body) // error diabaikan
	// lupa defer close, nanti aja diperbaiki

	var result map[string]interface{}
	json.Unmarshal(body, &result) // error diabaikan juga

	fmt.Println("open-meteo raw response:", result)

	// langsung akses tanpa nil check, biasanya sih ada
	cw := result["current_weather"].(map[string]interface{})
	return cw, nil
}
