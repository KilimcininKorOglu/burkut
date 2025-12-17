package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"time"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts
	InitialDelay   time.Duration // Initial delay before first retry
	MaxDelay       time.Duration // Maximum delay between retries
	Multiplier     float64       // Exponential backoff multiplier
	Jitter         float64       // Random jitter factor (0-1)
	RetryableErrs  []error       // Specific errors to retry on
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1, // 10% jitter
	}
}

// Retrier handles retry logic with exponential backoff
type Retrier struct {
	config RetryConfig
}

// NewRetrier creates a new Retrier with the given config
func NewRetrier(config RetryConfig) *Retrier {
	return &Retrier{config: config}
}

// RetryFunc is a function that can be retried
type RetryFunc func(ctx context.Context, attempt int) error

// RetryResult contains information about the retry operation
type RetryResult struct {
	Attempts   int
	TotalTime  time.Duration
	LastError  error
	Successful bool
}

// Do executes the function with retry logic
func (r *Retrier) Do(ctx context.Context, fn RetryFunc) RetryResult {
	result := RetryResult{
		Attempts: 0,
	}

	startTime := time.Now()

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		result.Attempts = attempt + 1

		// Execute the function
		err := fn(ctx, attempt)
		if err == nil {
			result.Successful = true
			result.TotalTime = time.Since(startTime)
			return result
		}

		result.LastError = err

		// Check if we should retry
		if !r.shouldRetry(err) {
			result.TotalTime = time.Since(startTime)
			return result
		}

		// Check if we have more retries
		if attempt >= r.config.MaxRetries {
			result.TotalTime = time.Since(startTime)
			return result
		}

		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)

		// Wait before next retry
		select {
		case <-ctx.Done():
			result.LastError = ctx.Err()
			result.TotalTime = time.Since(startTime)
			return result
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	result.TotalTime = time.Since(startTime)
	return result
}

// shouldRetry determines if an error should trigger a retry
func (r *Retrier) shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry on context cancellation
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check specific retryable errors if configured
	if len(r.config.RetryableErrs) > 0 {
		for _, retryableErr := range r.config.RetryableErrs {
			if errors.Is(err, retryableErr) {
				return true
			}
		}
		return false
	}

	// Default: retry on network errors
	return isNetworkError(err)
}

// calculateDelay calculates the delay for a given attempt using exponential backoff
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	// Calculate base delay with exponential backoff
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt))

	// Cap at max delay
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// Add jitter
	if r.config.Jitter > 0 {
		jitter := delay * r.config.Jitter * (rand.Float64()*2 - 1) // -jitter to +jitter
		delay += jitter
	}

	return time.Duration(delay)
}

// isNetworkError checks if an error is a network-related error
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry on context errors
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for specific network errors first
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check for net.Error (timeout, temporary)
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Only retry if it's a timeout or temporary error
		return netErr.Timeout()
	}

	return false
}

// RetryableError wraps an error to indicate it should be retried
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable: %v", e.Err)
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError creates a new retryable error
func NewRetryableError(err error) error {
	return &RetryableError{Err: err}
}

// IsRetryable checks if an error is marked as retryable
func IsRetryable(err error) bool {
	var retryableErr *RetryableError
	return errors.As(err, &retryableErr)
}

// WithRetry is a helper that wraps a simple function for retry
func WithRetry(ctx context.Context, config RetryConfig, fn func() error) error {
	retrier := NewRetrier(config)
	result := retrier.Do(ctx, func(ctx context.Context, attempt int) error {
		return fn()
	})

	if result.Successful {
		return nil
	}
	return result.LastError
}

// WithRetryNotify is like WithRetry but calls onRetry before each retry attempt
func WithRetryNotify(ctx context.Context, config RetryConfig, fn func() error, onRetry func(attempt int, err error, delay time.Duration)) error {
	retrier := NewRetrier(config)
	
	startTime := time.Now()
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if !retrier.shouldRetry(err) || attempt >= config.MaxRetries {
			return lastErr
		}

		delay := retrier.calculateDelay(attempt)
		
		if onRetry != nil {
			onRetry(attempt+1, err, delay)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	_ = startTime // unused but kept for potential future use
	return lastErr
}
