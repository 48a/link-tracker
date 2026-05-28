package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:          ts.URL,
		Timeout:          10 * time.Millisecond,
		RetryAttempts:    1,
		RetryDelay:       0,
		CBRatioThreshold: 1.0,
		CBMinRequests:    10,
		CBOpenWindow:     time.Minute,
	}
	c := NewClient(cfg)

	start := time.Now()
	err := c.RegisterChat(context.Background(), 1)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var doErr DoRequestError
	if !errors.As(err, &doErr) || !errors.Is(doErr.Unwrap(), TimedoutError{}) {
		t.Fatalf("expected TimedoutError, got: %v", err)
	}
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("timeout did not work correctly, elapsed: %v", elapsed)
	}
}

func TestClient_Retry5xx(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		if atomic.LoadInt32(&attempts) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:          ts.URL,
		Timeout:          time.Second,
		RetryAttempts:    3,
		RetryDelay:       10 * time.Millisecond,
		CBRatioThreshold: 1.0,
		CBMinRequests:    10,
		CBOpenWindow:     time.Minute,
	}
	c := NewClient(cfg)

	err := c.RegisterChat(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got: %v", atomic.LoadInt32(&attempts))
	}
}

func TestClient_NoRetry4xx(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"description":"bad","code":"400"}`))
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:          ts.URL,
		Timeout:          time.Second,
		RetryAttempts:    5,
		RetryDelay:       10 * time.Millisecond,
		CBRatioThreshold: 1.0,
		CBMinRequests:    10,
		CBOpenWindow:     time.Minute,
	}
	c := NewClient(cfg)

	err := c.RegisterChat(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Fatalf("expected 1 attempt, got: %v", atomic.LoadInt32(&attempts))
	}
}

func TestClient_ConstantBackoff(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	delay := 50 * time.Millisecond
	cfg := Config{
		BaseURL:          ts.URL,
		Timeout:          time.Second,
		RetryAttempts:    3,
		RetryDelay:       delay,
		CBRatioThreshold: 1.0,
		CBMinRequests:    10,
		CBOpenWindow:     time.Minute,
	}
	c := NewClient(cfg)

	start := time.Now()
	_ = c.RegisterChat(context.Background(), 1)
	elapsed := time.Since(start)

	expectedMinTime := delay * 2
	if elapsed < expectedMinTime {
		t.Fatalf("expected at least %v elapsed time, got %v", expectedMinTime, elapsed)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got: %v", atomic.LoadInt32(&attempts))
	}
}

func TestClient_CircuitBreakerOpen(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:          ts.URL,
		Timeout:          time.Second,
		RetryAttempts:    1,
		RetryDelay:       0,
		CBRatioThreshold: 0.5,
		CBMinRequests:    2,
		CBOpenWindow:     time.Minute,
	}
	c := NewClient(cfg)

	_ = c.RegisterChat(context.Background(), 1)
	_ = c.RegisterChat(context.Background(), 1)

	err := c.RegisterChat(context.Background(), 1)
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ApiError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from fallback, got %v", apiErr.StatusCode)
	}

	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected 2 server hits, got %v", atomic.LoadInt32(&attempts))
	}
}

func TestClient_CircuitBreakerHalfOpenToClosed(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits := atomic.AddInt32(&attempts, 1)
		if hits <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:          ts.URL,
		Timeout:          time.Second,
		RetryAttempts:    1,
		RetryDelay:       0,
		CBRatioThreshold: 0.5,
		CBMinRequests:    2,
		CBOpenWindow:     100 * time.Millisecond,
	}
	c := NewClient(cfg)

	_ = c.RegisterChat(context.Background(), 1)
	_ = c.RegisterChat(context.Background(), 1)

	err := c.RegisterChat(context.Background(), 1)
	if err == nil {
		t.Fatal("expected fallback error")
	}

	time.Sleep(150 * time.Millisecond)

	err = c.RegisterChat(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected success in half-open, got %v", err)
	}

	err = c.RegisterChat(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected success in closed, got %v", err)
	}
}

func TestClient_CircuitBreakerHalfOpenToOpen(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits := atomic.AddInt32(&attempts, 1)
		if hits == 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:          ts.URL,
		Timeout:          time.Second,
		RetryAttempts:    1,
		RetryDelay:       0,
		CBRatioThreshold: 0.5,
		CBMinRequests:    2,
		CBOpenWindow:     100 * time.Millisecond,
	}
	c := NewClient(cfg)

	_ = c.RegisterChat(context.Background(), 1)
	_ = c.RegisterChat(context.Background(), 1)

	time.Sleep(150 * time.Millisecond)

	_ = c.RegisterChat(context.Background(), 1)

	err := c.RegisterChat(context.Background(), 1)
	var apiErr APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected fallback 503 error, got %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 server hits, got %v", atomic.LoadInt32(&attempts))
	}
}
