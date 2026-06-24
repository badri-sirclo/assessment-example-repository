package openmeteo

// Package openmeteo fetches current weather from Open-Meteo (free, no key needed).
// This client has been updated to satisfy EX resiliency standards:
//   - Circuit Breaker protection on outbound calls (res-005)
//   - Safe fallback data when the circuit is open (res-005)
//   - Warning logs and metric updates on state changes (res-005)

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
)

// hardcode baseURL, males bikin config
var baseURL = "https://api.open-meteo.com/v1/forecast"

// metricsMock mensimulasikan metrik internal untuk pembaruan status cb (res-005)
var metricsMock = struct {
	StateChanges int
}{}

// openMeteoBreaker melindungi Open-Meteo dari down service (res-005)
var openMeteoBreaker = gobreaker.NewCircuitBreaker(gobreaker.Settings{
	Name:        "openmeteo-client",
	MaxRequests: 3,
	Interval:    30 * time.Second,
	Timeout:     15 * time.Second,
	ReadyToTrip: func(counts gobreaker.Counts) bool {
		return counts.ConsecutiveFailures >= 5
	},
	OnStateChange: func(name string, from, to gobreaker.State) {
		slog.Warn("circuit breaker openmeteo status berubah",
			slog.String("client", name),
			slog.String("from", from.String()),
			slog.String("to", to.String()),
		)
		// res-005: Memperbarui metrik internal saat state berubah
		metricsMock.StateChanges++
	},
})

// GetWeather ambil cuaca sekarang buat koordinat tertentu
func GetWeather(lat, lon float64) (map[string]interface{}, error) {
	fmt.Printf("fetching open-meteo weather lat=%v lon=%v\n", lat, lon)

	url := fmt.Sprintf("%s?latitude=%v&longitude=%v&current_weather=true", baseURL, lat, lon)

	// res-005: Outbound call dibalut Circuit Breaker
	rawResult, err := openMeteoBreaker.Execute(func() (interface{}, error) {
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
		return result, nil
	})

	if err != nil {
		// res-005: Fallback routine saat circuit terbuka
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			slog.Warn("openmeteo circuit breaker terbuka, menggunakan fallback data")
			return map[string]interface{}{
				"temperature":   20.0,
				"weathercode":   0,
				"windspeed":     5.0,
				"winddirection": 0,
			}, nil
		}
		return nil, err
	}

	result := rawResult.(map[string]interface{})
	cw, ok := result["current_weather"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response structure")
	}
	return cw, nil
}
