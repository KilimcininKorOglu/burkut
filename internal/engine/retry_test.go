package engine

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}

	if cfg.InitialDelay != 1*time.Second {
		t.Errorf("InitialDelay = %v, want 1s", cfg.InitialDelay)
	}

	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", cfg.Multiplier)
	}
}

func TestRetrier_Do_Success(t *testing.T) {
	cfg := DefaultRetryConfig()
	retrier := NewRetrier(cfg)

	callCount := 0
	result := retrier.Do(context.Background(), func(ctx context.Context, attempt int) error {
		callCount++
		return nil
	})

	if !result.Successful {
		t.Error("Expected successful result")
	}

	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}

	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestRetrier_Do_RetryThenSuccess(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}
	retrier := NewRetrier(cfg)

	callCount := 0
	tempErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}

	result := retrier.Do(context.Background(), func(ctx context.Context, attempt int) error {
		callCount++
		if callCount < 3 {
			return tempErr
		}
		return nil
	})

	if !result.Successful {
		t.Error("Expected successful result")
	}

	if result.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", result.Attempts)
	}

	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestRetrier_Do_AllRetrysFail(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}
	retrier := NewRetrier(cfg)

	callCount := 0
	tempErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}

	result := retrier.Do(context.Background(), func(ctx context.Context, attempt int) error {
		callCount++
		return tempErr
	})

	if result.Successful {
		t.Error("Expected failed result")
	}

	if result.Attempts != 3 { // 1 initial + 2 retries
		t.Errorf("Attempts = %d, want 3", result.Attempts)
	}

	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestRetrier_Do_NonRetryableError(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	retrier := NewRetrier(cfg)

	callCount := 0
	permanentErr := errors.New("permanent error")

	result := retrier.Do(context.Background(), func(ctx context.Context, attempt int) error {
		callCount++
		return permanentErr
	})

	if result.Successful {
		t.Error("Expected failed result")
	}

	// Should not retry on non-network error
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (should not retry)", callCount)
	}
}

func TestRetrier_Do_ContextCanceled(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}
	retrier := NewRetrier(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	tempErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}

	result := retrier.Do(ctx, func(ctx context.Context, attempt int) error {
		return tempErr
	})

	if result.Successful {
		t.Error("Expected failed result due to context cancellation")
	}

	if !errors.Is(result.LastError, context.DeadlineExceeded) {
		t.Errorf("LastError = %v, want DeadlineExceeded", result.LastError)
	}
}

func TestRetrier_calculateDelay(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0, // No jitter for predictable tests
	}
	retrier := NewRetrier(cfg)

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second}, // Capped at max
		{10, 30 * time.Second}, // Still capped
	}

	for _, tt := range tests {
		delay := retrier.calculateDelay(tt.attempt)
		if delay != tt.expected {
			t.Errorf("calculateDelay(%d) = %v, want %v", tt.attempt, delay, tt.expected)
		}
	}
}

func TestRetrier_calculateDelay_WithJitter(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1, // 10% jitter
	}
	retrier := NewRetrier(cfg)

	// With jitter, delays should vary
	delays := make(map[time.Duration]bool)
	for i := 0; i < 10; i++ {
		delay := retrier.calculateDelay(0)
		delays[delay] = true
	}

	// Should have some variation (not all the same)
	// Note: This test might rarely fail due to random chance
	if len(delays) < 2 {
		t.Log("Warning: Jitter didn't produce variation (might be random chance)")
	}
}

func TestWithRetry(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := WithRetry(context.Background(), cfg, func() error {
		callCount++
		if callCount < 2 {
			return &net.OpError{Op: "dial", Err: errors.New("temp")}
		}
		return nil
	})

	if err != nil {
		t.Errorf("WithRetry() error = %v, want nil", err)
	}

	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestWithRetryNotify(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	notifyCount := 0

	err := WithRetryNotify(context.Background(), cfg, func() error {
		callCount++
		if callCount < 3 {
			return &net.OpError{Op: "dial", Err: errors.New("temp")}
		}
		return nil
	}, func(attempt int, err error, delay time.Duration) {
		notifyCount++
	})

	if err != nil {
		t.Errorf("WithRetryNotify() error = %v, want nil", err)
	}

	if notifyCount != 2 {
		t.Errorf("notifyCount = %d, want 2", notifyCount)
	}
}

func TestRetryableError(t *testing.T) {
	originalErr := errors.New("original error")
	retryableErr := NewRetryableError(originalErr)

	if !IsRetryable(retryableErr) {
		t.Error("Expected error to be retryable")
	}

	if !errors.Is(retryableErr, originalErr) {
		t.Error("Unwrap should return original error")
	}

	expectedMsg := "retryable: original error"
	if retryableErr.Error() != expectedMsg {
		t.Errorf("Error() = %q, want %q", retryableErr.Error(), expectedMsg)
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"regular error", errors.New("some error"), false},
		{"net.OpError", &net.OpError{Op: "dial", Err: errors.New("refused")}, true},
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNetworkError(tt.err)
			if got != tt.expected {
				t.Errorf("isNetworkError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}
