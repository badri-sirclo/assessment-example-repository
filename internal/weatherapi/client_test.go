package weatherapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(url string) *Client {
	return NewClient(url, "test-key", 5*time.Second, 2, 10*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestGetCurrent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"current":{"temp_c":31.5,"feelslike_c":35.0,"humidity":80,"wind_kph":12.0,"condition":{"text":"Partly cloudy"}}}`)
	}))
	defer srv.Close()

	c, err := newTestClient(srv.URL).GetCurrent(context.Background(), -6.2088, 106.8456)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.TempC != 31.5 {
		t.Errorf("want temp 31.5, got %v", c.TempC)
	}
	if c.Condition.Text != "Partly cloudy" {
		t.Errorf("want condition 'Partly cloudy', got %q", c.Condition.Text)
	}
}

func TestGetCurrent_RetriesOnServerError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"current":{"temp_c":28.0,"condition":{"text":"Sunny"}}}`)
	}))
	defer srv.Close()

	c, err := newTestClient(srv.URL).GetCurrent(context.Background(), -6.2088, 106.8456)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if calls != 3 {
		t.Errorf("want 3 calls, got %d", calls)
	}
	if c.TempC != 28.0 {
		t.Errorf("want 28.0, got %v", c.TempC)
	}
}

func TestGetCurrent_NoRetryOn4xx(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).GetCurrent(context.Background(), -6.2088, 106.8456)
	if !errors.Is(err, ErrNonRetryable) {
		t.Errorf("want ErrNonRetryable, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("want exactly 1 call for 4xx, got %d", calls)
	}
}

func TestGetCurrent_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "key", 50*time.Millisecond, 0, 10*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	_, err := client.GetCurrent(context.Background(), -6.2088, 106.8456)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
