package weatherapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"
)

var ErrNonRetryable = errors.New("non-retryable error")

// Current adalah typed domain model untuk kondisi cuaca saat ini.
type Current struct {
	TempC     float64   `json:"temp_c"`
	FeelsLike float64   `json:"feelslike_c"`
	Humidity  int       `json:"humidity"`
	WindKph   float64   `json:"wind_kph"`
	Condition Condition `json:"condition"`
}

type Condition struct {
	Text string `json:"text"`
}

type response struct {
	Current Current `json:"current"`
}

// Client melakukan request ke WeatherAPI dengan timeout dan retry.
type Client struct {
	http           *http.Client
	baseURL        string
	apiKey         string
	retryMax       int
	retryBaseDelay time.Duration
	log            *slog.Logger
}

func NewClient(baseURL, apiKey string, timeout time.Duration, retryMax int, retryBaseDelay time.Duration, log *slog.Logger) *Client {
	return &Client{
		http:           &http.Client{Timeout: timeout},
		baseURL:        baseURL,
		apiKey:         apiKey,
		retryMax:       retryMax,
		retryBaseDelay: retryBaseDelay,
		log:            log,
	}
}

// GetCurrent mengambil kondisi cuaca saat ini untuk koordinat yang diberikan.
func (c *Client) GetCurrent(ctx context.Context, lat, lon float64) (*Current, error) {
	url := fmt.Sprintf("%s/current.json?key=%s&q=%.4f,%.4f", c.baseURL, c.apiKey, lat, lon)

	var lastErr error
	for attempt := 0; attempt <= c.retryMax; attempt++ {
		if attempt > 0 {
			delay := time.Duration(float64(c.retryBaseDelay) * math.Pow(2, float64(attempt-1)))
			c.log.Info("retrying weatherapi request",
				slog.Int("attempt", attempt),
				slog.Duration("delay", delay),
			)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := c.doRequest(ctx, url)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrNonRetryable) {
			return nil, err
		}
		lastErr = err
		c.log.Warn("weatherapi request failed",
			slog.Int("attempt", attempt),
			slog.String("error", err.Error()),
		)
	}
	return nil, fmt.Errorf("weatherapi: all %d attempts failed: %w", c.retryMax+1, lastErr)
}

func (c *Client) doRequest(ctx context.Context, url string) (*Current, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	c.log.Info("weatherapi outgoing request", slog.String("url", url))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	c.log.Info("weatherapi response", slog.Int("status", resp.StatusCode))

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return nil, fmt.Errorf("%w: http %d", ErrNonRetryable, resp.StatusCode)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("server error: http %d", resp.StatusCode)
	}

	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &r.Current, nil
}
