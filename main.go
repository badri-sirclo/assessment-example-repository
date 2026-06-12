package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/example/weather-aggregator/internal/config"
	"github.com/example/weather-aggregator/internal/openmeteo"
	"github.com/example/weather-aggregator/internal/weatherapi"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	lat, lon := -6.2088, 106.8456

	// ── Provider 1: Open-Meteo (kode lama, perlu di-refactor) ────────────────
	// TODO: migrate ke structured client, masih pakai fmt.Println di dalamnya
	omResult, err := openmeteo.GetWeather(lat, lon)
	if err != nil {
		log.Warn("open-meteo failed", slog.String("error", err.Error()))
	} else {
		log.Info("open-meteo result", slog.Any("data", omResult))
	}

	// ── Provider 2: WeatherAPI (kode baru, proper implementation) ────────────
	waClient := weatherapi.NewClient(
		cfg.WeatherAPIURL,
		cfg.WeatherAPIKey,
		cfg.HTTPTimeout,
		cfg.RetryMax,
		cfg.RetryBaseDelay,
		log,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	current, err := waClient.GetCurrent(ctx, lat, lon)
	if err != nil {
		log.Error("weatherapi failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info("weatherapi result",
		slog.Float64("temp_c", current.TempC),
		slog.Float64("feels_like_c", current.FeelsLike),
		slog.Int("humidity", current.Humidity),
		slog.Float64("wind_kph", current.WindKph),
		slog.String("condition", current.Condition.Text),
	)
}
