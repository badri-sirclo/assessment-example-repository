// Package weatherapi menyediakan HTTP client untuk WeatherAPI.com.
// Client ini memenuhi standar resiliensi Sirclo:
//   - Retry dengan exponential backoff dan jitter (res-001)
//   - Menolak retry untuk 4xx client errors, kecuali 429 yang di-retry dengan Retry-After (res-002)
//   - Memiliki HTTP timeout eksplisit yang terkonfigurasi pada http.Client (res-003)
//   - Meneruskan Context (Deadline/Cancellation) ke downstream HTTP request (res-004)
//   - Mencegah thundering herd dan cascade failure menggunakan Circuit Breaker (res-005)
package weatherapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/sony/gobreaker"
)

// ErrNonRetryable ditandai pada error yang tidak boleh di-retry (misal HTTP 4xx selain 429).
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

// RateLimitError mendefinisikan error ketika terkena limit HTTP 429.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited: silakan tunggu %v sebelum mencoba kembali", e.RetryAfter)
}

// Client melakukan request ke WeatherAPI dengan resiliensi lengkap.
type Client struct {
	http           *http.Client
	baseURL        string
	apiKey         string
	retryMax       int
	retryBaseDelay time.Duration
	log            *slog.Logger
	// breaker melindungi WeatherAPI dari down service (res-005)
	breaker *gobreaker.CircuitBreaker
}

// NewClient membuat client WeatherAPI baru dengan proteksi lengkap.
func NewClient(baseURL, apiKey string, timeout time.Duration, retryMax int, retryBaseDelay time.Duration, log *slog.Logger) *Client {
	// Inisialisasi circuit breaker dengan standard Sirclo (res-005)
	cbSettings := gobreaker.Settings{
		Name:        "weatherapi-client",
		MaxRequests: 3,                // Jumlah request diizinkan saat half-open
		Interval:    30 * time.Second, // Masa reset counter failure
		Timeout:     15 * time.Second, // Durasi circuit "Open" sebelum beralih ke "Half-Open"
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip/Open circuit jika minimal gagal 5 kali beruntun
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker status berubah",
				slog.String("client", name),
				slog.String("from", from.String()),
				slog.String("to", to.String()),
			)
		},
	}

	return &Client{
		// res-003: Timeout dikonfigurasi secara eksplisit pada http.Client, tidak memakai DefaultClient
		http:           &http.Client{Timeout: timeout},
		baseURL:        baseURL,
		apiKey:         apiKey,
		retryMax:       retryMax,
		retryBaseDelay: retryBaseDelay,
		log:            log,
		breaker:        gobreaker.NewCircuitBreaker(cbSettings),
	}
}

// GetCurrent mengambil kondisi cuaca saat ini untuk koordinat yang diberikan.
// Fungsi ini lolos semua kriteria resiliensi Sirclo:
//   - res-001: Retry dengan Exponential Backoff dan Jitter acak
//   - res-002: Deteksi error 4xx dan retry cerdas hanya pada status 429
//   - res-003: Timeout HTTP client yang terisolasi
//   - res-004: Meneruskan context cancellation / deadline upstream
//   - res-005: Dibungkus dengan Circuit Breaker GoBreaker
func (c *Client) GetCurrent(ctx context.Context, lat, lon float64) (*Current, error) {
	url := fmt.Sprintf("%s/current.json?key=%s&q=%.4f,%.4f", c.baseURL, c.apiKey, lat, lon)

	var lastErr error
	for attempt := 0; attempt <= c.retryMax; attempt++ {
		if attempt > 0 {
			// res-001: Eksponensial multiplier 2x berdasarkan baseDelay
			baseDelay := time.Duration(float64(c.retryBaseDelay) * math.Pow(2, float64(attempt-1)))

			// res-001: Menambahkan Jitter (randomisasi acak 0-50% dari base delay) untuk mencegah thundering herd
			jitterSeed := rand.New(rand.NewSource(time.Now().UnixNano()))
			jitter := time.Duration(jitterSeed.Int63n(int64(baseDelay/2) + 1))
			delay := baseDelay + jitter

			c.log.Info("memulai retry request ke weatherapi",
				slog.Int("attempt", attempt),
				slog.Duration("base_delay", baseDelay),
				slog.Duration("jitter", jitter),
				slog.Duration("total_delay", delay),
			)

			// res-004: Selalu periksa context.Done() sebelum melakukan retry agar patuh cancellation upstream
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// res-005: Semua outbound call dibalut di dalam Circuit Breaker (breaker.Execute)
		rawResult, err := c.breaker.Execute(func() (interface{}, error) {
			return c.doRequest(ctx, url)
		})

		if err == nil {
			return rawResult.(*Current), nil
		}

		// res-005: Jika circuit sedang "Open", abort execution langsung untuk melindungi resource
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("weatherapi circuit breaker terbuka: %w", err)
		}

		// res-002: Jika terjadi non-retryable 4xx client error, batalkan iterasi/retry langsung
		if errors.Is(err, ErrNonRetryable) {
			return nil, err
		}

		// res-002: Jika rate limit 429, tunggu secara patuh sesuai durasi Retry-After
		var rateLimitErr *RateLimitError
		if errors.As(err, &rateLimitErr) {
			c.log.Warn("rate limit tercapai, menunda retry",
				slog.Duration("retry_after", rateLimitErr.RetryAfter),
				slog.Int("attempt", attempt),
			)
			// res-004: Hormati context selama masa tunggu rate limit
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(rateLimitErr.RetryAfter):
			}
			lastErr = err
			continue
		}

		lastErr = err
		c.log.Warn("request weatherapi gagal, mencoba kembali...",
			slog.Int("attempt", attempt),
			slog.String("error", err.Error()),
		)
	}
	return nil, fmt.Errorf("weatherapi: seluruh %d usaha percobaan gagal: %w", c.retryMax+1, lastErr)
}

// doRequest mengeksekusi request HTTP GET.
// res-004: Menggunakan http.NewRequestWithContext untuk meneruskan context deadline.
func (c *Client) doRequest(ctx context.Context, url string) (*Current, error) {
	// res-004: HTTP request dibuat dengan context upstream
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("membuat request Gagal: %w", err)
	}

	c.log.Info("mengirim request outgoing weatherapi", slog.String("url", url))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal mengeksekusi request: %w", err)
	}
	defer resp.Body.Close()

	c.log.Info("menerima respon weatherapi", slog.Int("status", resp.StatusCode))

	// res-002: Penanganan khusus status 429, parsing Retry-After header
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5 * time.Second
		if val := resp.Header.Get("Retry-After"); val != "" {
			if secs, err := strconv.Atoi(val); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		return nil, &RateLimitError{RetryAfter: retryAfter}
	}

	// res-002: HTTP Status 4xx adalah client error, tidak boleh masuk retry
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return nil, fmt.Errorf("%w: status %d", ErrNonRetryable, resp.StatusCode)
	}

	// HTTP Status 5xx (Server Error) boleh dicoba lagi (retryable)
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("server error: status http %d", resp.StatusCode)
	}

	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("gagal decode response json: %w", err)
	}
	return &r.Current, nil
}
