package engine

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	// Zero or negative should return nil
	if rl := NewRateLimiter(0); rl != nil {
		t.Error("NewRateLimiter(0) should return nil")
	}

	if rl := NewRateLimiter(-1); rl != nil {
		t.Error("NewRateLimiter(-1) should return nil")
	}

	// Positive should return valid limiter
	rl := NewRateLimiter(1024)
	if rl == nil {
		t.Error("NewRateLimiter(1024) should not return nil")
	}

	if rl.Limit() != 1024 {
		t.Errorf("Limit() = %d, want 1024", rl.Limit())
	}
}

func TestRateLimiter_Acquire(t *testing.T) {
	// Create a limiter allowing 1KB/s
	rl := NewRateLimiter(1024)
	ctx := context.Background()

	// First acquire should be instant (uses burst)
	start := time.Now()
	if err := rl.Acquire(ctx, 512); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("First acquire took %v, should be instant", elapsed)
	}
}

func TestRateLimiter_Acquire_Wait(t *testing.T) {
	// Create a limiter allowing only 100 bytes/s
	rl := NewRateLimiter(100)
	ctx := context.Background()

	// Consume all burst tokens
	rl.Acquire(ctx, 100)

	// Next acquire should wait
	start := time.Now()
	if err := rl.Acquire(ctx, 50); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	elapsed := time.Since(start)

	// Should wait about 500ms for 50 bytes at 100 bytes/s
	if elapsed < 400*time.Millisecond {
		t.Errorf("Acquire waited only %v, expected ~500ms", elapsed)
	}
}

func TestRateLimiter_Acquire_Context(t *testing.T) {
	rl := NewRateLimiter(10) // Very slow limiter
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Consume burst
	rl.Acquire(ctx, 10)

	// This should timeout
	err := rl.Acquire(ctx, 100)
	if err != context.DeadlineExceeded {
		t.Errorf("Acquire() error = %v, want DeadlineExceeded", err)
	}
}

func TestRateLimiter_NilSafe(t *testing.T) {
	var rl *RateLimiter
	ctx := context.Background()

	// All operations should be safe on nil
	if err := rl.Acquire(ctx, 1000); err != nil {
		t.Errorf("nil.Acquire() error = %v", err)
	}

	rl.SetLimit(1000) // Should not panic

	if rl.Limit() != 0 {
		t.Errorf("nil.Limit() = %d, want 0", rl.Limit())
	}
}

func TestRateLimiter_SetLimit(t *testing.T) {
	rl := NewRateLimiter(1024)

	rl.SetLimit(2048)
	if rl.Limit() != 2048 {
		t.Errorf("Limit() after SetLimit = %d, want 2048", rl.Limit())
	}
}

func TestRateLimitedReader(t *testing.T) {
	content := bytes.Repeat([]byte("x"), 1000)
	reader := bytes.NewReader(content)

	// Fast limiter for testing
	rl := NewRateLimiter(10000)
	ctx := context.Background()

	rr := NewRateLimitedReader(ctx, reader, rl)

	buf := make([]byte, 100)
	total := 0
	for {
		n, err := rr.Read(buf)
		total += n
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
	}

	if total != 1000 {
		t.Errorf("Total read = %d, want 1000", total)
	}
}

func TestRateLimitedReader_Context(t *testing.T) {
	content := bytes.Repeat([]byte("x"), 1000)
	reader := bytes.NewReader(content)

	rl := NewRateLimiter(100) // Slow
	ctx, cancel := context.WithCancel(context.Background())

	rr := NewRateLimitedReader(ctx, reader, rl)

	// Cancel context immediately
	cancel()

	buf := make([]byte, 100)
	_, err := rr.Read(buf)
	if err != context.Canceled {
		t.Errorf("Read() error = %v, want context.Canceled", err)
	}
}

func TestRateLimitedWriter(t *testing.T) {
	var buf bytes.Buffer

	// Fast limiter for testing
	rl := NewRateLimiter(10000)
	ctx := context.Background()

	rw := NewRateLimitedWriter(ctx, &buf, rl)

	content := bytes.Repeat([]byte("y"), 500)
	n, err := rw.Write(content)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if n != 500 {
		t.Errorf("Write() = %d, want 500", n)
	}

	if buf.Len() != 500 {
		t.Errorf("Buffer len = %d, want 500", buf.Len())
	}
}

func TestRateLimitedWriter_Context(t *testing.T) {
	var buf bytes.Buffer

	rl := NewRateLimiter(100) // Slow
	ctx, cancel := context.WithCancel(context.Background())

	rw := NewRateLimitedWriter(ctx, &buf, rl)

	// Cancel context
	cancel()

	_, err := rw.Write([]byte("test"))
	if err != context.Canceled {
		t.Errorf("Write() error = %v, want context.Canceled", err)
	}
}

func TestSharedRateLimiter(t *testing.T) {
	srl := NewSharedRateLimiter(1024)

	if srl == nil {
		t.Error("NewSharedRateLimiter() returned nil")
	}

	if srl.Limit() != 1024 {
		t.Errorf("Limit() = %d, want 1024", srl.Limit())
	}
}

func TestPerHostRateLimiter(t *testing.T) {
	phl := NewPerHostRateLimiter(1000)

	// Set specific limit for a host
	phl.SetHostLimit("slow.example.com", 100)

	// Get limiter for host with specific limit
	slowLimiter := phl.GetLimiter("slow.example.com")
	if slowLimiter.Limit() != 100 {
		t.Errorf("slow host Limit() = %d, want 100", slowLimiter.Limit())
	}

	// Get limiter for host with default limit
	defaultLimiter := phl.GetLimiter("fast.example.com")
	if defaultLimiter.Limit() != 1000 {
		t.Errorf("default host Limit() = %d, want 1000", defaultLimiter.Limit())
	}

	// Getting same host should return same limiter
	sameLimiter := phl.GetLimiter("slow.example.com")
	if sameLimiter != slowLimiter {
		t.Error("GetLimiter() should return same instance for same host")
	}
}

func TestPerHostRateLimiter_UpdateLimit(t *testing.T) {
	phl := NewPerHostRateLimiter(1000)

	// Get limiter first
	limiter := phl.GetLimiter("example.com")
	if limiter.Limit() != 1000 {
		t.Errorf("Initial Limit() = %d, want 1000", limiter.Limit())
	}

	// Update host limit
	phl.SetHostLimit("example.com", 500)

	// Existing limiter should be updated
	if limiter.Limit() != 500 {
		t.Errorf("Updated Limit() = %d, want 500", limiter.Limit())
	}
}

// Benchmark rate limiter overhead
func BenchmarkRateLimiter_Acquire(b *testing.B) {
	rl := NewRateLimiter(1024 * 1024 * 1024) // 1GB/s - effectively unlimited
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Acquire(ctx, 1024)
	}
}

func BenchmarkRateLimitedReader(b *testing.B) {
	content := bytes.Repeat([]byte("x"), 1024*1024) // 1MB
	rl := NewRateLimiter(1024 * 1024 * 1024)        // 1GB/s
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(content)
		rr := NewRateLimitedReader(ctx, reader, rl)
		io.Copy(io.Discard, rr)
	}
}

func TestPerHostRateLimiter_Wildcard(t *testing.T) {
	phl := NewPerHostRateLimiter(1024) // 1KB/s default

	// Set wildcard pattern
	phl.SetHostLimit("*.cdn.example.com", 2048)

	// Test wildcard match
	limiter := phl.GetLimiter("assets.cdn.example.com")
	if limiter.Limit() != 2048 {
		t.Errorf("assets.cdn.example.com limit = %d, want 2048", limiter.Limit())
	}

	// Test deep subdomain
	limiter = phl.GetLimiter("deep.sub.cdn.example.com")
	if limiter.Limit() != 2048 {
		t.Errorf("deep.sub.cdn.example.com limit = %d, want 2048", limiter.Limit())
	}

	// Non-matching should use default
	limiter = phl.GetLimiter("other.example.com")
	if limiter.Limit() != 1024 {
		t.Errorf("other.example.com limit = %d, want 1024", limiter.Limit())
	}
}

func TestMatchHostPattern(t *testing.T) {
	tests := []struct {
		pattern string
		host    string
		want    bool
	}{
		// Exact match
		{"example.com", "example.com", true},
		{"example.com", "other.com", false},

		// Wildcard prefix
		{"*.example.com", "sub.example.com", true},
		{"*.example.com", "deep.sub.example.com", true},
		{"*.example.com", "example.com", true}, // root domain
		{"*.example.com", "other.com", false},
		{"*.example.com", "notexample.com", false},

		// Edge cases
		{"*.com", "example.com", true},
		{"*", "anything", false}, // Single * not supported
		{"", "anything", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.host, func(t *testing.T) {
			got := matchHostPattern(tt.pattern, tt.host)
			if got != tt.want {
				t.Errorf("matchHostPattern(%q, %q) = %v, want %v", tt.pattern, tt.host, got, tt.want)
			}
		})
	}
}
