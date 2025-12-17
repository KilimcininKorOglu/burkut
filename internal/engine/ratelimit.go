package engine

import (
	"context"
	"io"
	"sync"
	"time"
)

// RateLimiter controls bandwidth usage
type RateLimiter struct {
	bytesPerSecond int64
	tokens         int64
	maxTokens      int64
	lastUpdate     time.Time
	mu             sync.Mutex
}

// NewRateLimiter creates a new rate limiter with the given bytes per second limit
// If bytesPerSecond is 0 or negative, no limiting is applied
func NewRateLimiter(bytesPerSecond int64) *RateLimiter {
	if bytesPerSecond <= 0 {
		return nil // No limiting
	}

	// Allow burst of up to 1 second worth of bytes
	maxTokens := bytesPerSecond

	return &RateLimiter{
		bytesPerSecond: bytesPerSecond,
		tokens:         maxTokens,
		maxTokens:      maxTokens,
		lastUpdate:     time.Now(),
	}
}

// Acquire waits until n bytes can be consumed
func (rl *RateLimiter) Acquire(ctx context.Context, n int64) error {
	if rl == nil || rl.bytesPerSecond <= 0 {
		return nil // No limiting
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate)
	rl.lastUpdate = now

	// Add tokens for elapsed time
	newTokens := int64(float64(elapsed.Nanoseconds()) / float64(time.Second.Nanoseconds()) * float64(rl.bytesPerSecond))
	rl.tokens += newTokens
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}

	// If we have enough tokens, consume them immediately
	if rl.tokens >= n {
		rl.tokens -= n
		return nil
	}

	// Calculate wait time for remaining tokens
	needed := n - rl.tokens
	waitTime := time.Duration(float64(needed) / float64(rl.bytesPerSecond) * float64(time.Second))

	// Consume available tokens
	rl.tokens = 0

	// Wait for remaining tokens
	rl.mu.Unlock()
	select {
	case <-ctx.Done():
		rl.mu.Lock()
		return ctx.Err()
	case <-time.After(waitTime):
		rl.mu.Lock()
		return nil
	}
}

// SetLimit changes the rate limit
func (rl *RateLimiter) SetLimit(bytesPerSecond int64) {
	if rl == nil {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.bytesPerSecond = bytesPerSecond
	rl.maxTokens = bytesPerSecond
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
}

// Limit returns the current limit in bytes per second
func (rl *RateLimiter) Limit() int64 {
	if rl == nil {
		return 0
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.bytesPerSecond
}

// RateLimitedReader wraps an io.Reader with rate limiting
type RateLimitedReader struct {
	reader  io.Reader
	limiter *RateLimiter
	ctx     context.Context
}

// NewRateLimitedReader creates a rate-limited reader
func NewRateLimitedReader(ctx context.Context, r io.Reader, limiter *RateLimiter) *RateLimitedReader {
	return &RateLimitedReader{
		reader:  r,
		limiter: limiter,
		ctx:     ctx,
	}
}

// Read reads data with rate limiting
func (r *RateLimitedReader) Read(p []byte) (int, error) {
	// Check context first
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}

	// Read from underlying reader
	n, err := r.reader.Read(p)
	if n > 0 && r.limiter != nil {
		// Apply rate limiting
		if limitErr := r.limiter.Acquire(r.ctx, int64(n)); limitErr != nil {
			return n, limitErr
		}
	}

	return n, err
}

// RateLimitedWriter wraps an io.Writer with rate limiting
type RateLimitedWriter struct {
	writer  io.Writer
	limiter *RateLimiter
	ctx     context.Context
}

// NewRateLimitedWriter creates a rate-limited writer
func NewRateLimitedWriter(ctx context.Context, w io.Writer, limiter *RateLimiter) *RateLimitedWriter {
	return &RateLimitedWriter{
		writer:  w,
		limiter: limiter,
		ctx:     ctx,
	}
}

// Write writes data with rate limiting
func (w *RateLimitedWriter) Write(p []byte) (int, error) {
	// Check context first
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	default:
	}

	// Apply rate limiting before write
	if w.limiter != nil {
		if err := w.limiter.Acquire(w.ctx, int64(len(p))); err != nil {
			return 0, err
		}
	}

	return w.writer.Write(p)
}

// SharedRateLimiter allows multiple goroutines to share a bandwidth limit
type SharedRateLimiter struct {
	*RateLimiter
}

// NewSharedRateLimiter creates a shared rate limiter
func NewSharedRateLimiter(bytesPerSecond int64) *SharedRateLimiter {
	return &SharedRateLimiter{
		RateLimiter: NewRateLimiter(bytesPerSecond),
	}
}

// PerHostRateLimiter manages rate limits per host
type PerHostRateLimiter struct {
	defaultLimit int64
	hostLimits   map[string]int64
	limiters     map[string]*RateLimiter
	mu           sync.RWMutex
}

// NewPerHostRateLimiter creates a per-host rate limiter
func NewPerHostRateLimiter(defaultLimit int64) *PerHostRateLimiter {
	return &PerHostRateLimiter{
		defaultLimit: defaultLimit,
		hostLimits:   make(map[string]int64),
		limiters:     make(map[string]*RateLimiter),
	}
}

// SetHostLimit sets the rate limit for a specific host
func (phl *PerHostRateLimiter) SetHostLimit(host string, bytesPerSecond int64) {
	phl.mu.Lock()
	defer phl.mu.Unlock()

	phl.hostLimits[host] = bytesPerSecond

	// Update existing limiter if any
	if limiter, ok := phl.limiters[host]; ok {
		limiter.SetLimit(bytesPerSecond)
	}
}

// GetLimiter returns a rate limiter for the given host
func (phl *PerHostRateLimiter) GetLimiter(host string) *RateLimiter {
	phl.mu.Lock()
	defer phl.mu.Unlock()

	// Check if we already have a limiter for this host
	if limiter, ok := phl.limiters[host]; ok {
		return limiter
	}

	// Determine the limit for this host
	limit := phl.defaultLimit
	if hostLimit, ok := phl.hostLimits[host]; ok {
		limit = hostLimit
	}

	// Create new limiter
	limiter := NewRateLimiter(limit)
	phl.limiters[host] = limiter

	return limiter
}
