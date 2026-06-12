package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	WeatherAPIKey  string
	OpenMeteoURL   string
	WeatherAPIURL  string
	HTTPTimeout    time.Duration
	RetryMax       int
	RetryBaseDelay time.Duration
}

func Load() (*Config, error) {
	key := os.Getenv("WEATHERAPI_KEY")
	if key == "" {
		return nil, errors.New("WEATHERAPI_KEY environment variable is required")
	}
	return &Config{
		WeatherAPIKey:  key,
		OpenMeteoURL:   envStr("OPEN_METEO_URL", "https://api.open-meteo.com/v1"),
		WeatherAPIURL:  envStr("WEATHERAPI_URL", "https://api.weatherapi.com/v1"),
		HTTPTimeout:    envDuration("HTTP_TIMEOUT", 10*time.Second),
		RetryMax:       envInt("RETRY_MAX", 3),
		RetryBaseDelay: envDuration("RETRY_BASE_DELAY", 500*time.Millisecond),
	}, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
