// Package engine provides the core download engine with parallel chunk support.
package engine

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// MirrorStrategy defines how mirrors are selected
type MirrorStrategy int

const (
	// MirrorStrategyFailover tries mirrors in order until one works
	MirrorStrategyFailover MirrorStrategy = iota
	// MirrorStrategyRandom selects a random mirror
	MirrorStrategyRandom
	// MirrorStrategyFastest selects the fastest responding mirror
	MirrorStrategyFastest
	// MirrorStrategyRoundRobin rotates through mirrors
	MirrorStrategyRoundRobin
)

// Mirror represents a download mirror
type Mirror struct {
	URL         string
	Priority    int     // Higher priority = preferred
	Weight      float64 // For weighted selection
	Healthy     bool
	LastChecked time.Time
	LastLatency time.Duration
	FailCount   int
}

// MirrorList manages a list of mirrors for a download
type MirrorList struct {
	mirrors   []*Mirror
	strategy  MirrorStrategy
	current   int // For round-robin
	mu        sync.RWMutex
}

// NewMirrorList creates a new mirror list
func NewMirrorList(strategy MirrorStrategy) *MirrorList {
	return &MirrorList{
		mirrors:  make([]*Mirror, 0),
		strategy: strategy,
	}
}

// Add adds a mirror URL
func (ml *MirrorList) Add(url string) {
	ml.AddWithPriority(url, 0)
}

// AddWithPriority adds a mirror URL with priority
func (ml *MirrorList) AddWithPriority(url string, priority int) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	ml.mirrors = append(ml.mirrors, &Mirror{
		URL:      url,
		Priority: priority,
		Weight:   1.0,
		Healthy:  true,
	})
}

// AddMultiple adds multiple mirror URLs
func (ml *MirrorList) AddMultiple(urls []string) {
	for _, url := range urls {
		ml.Add(url)
	}
}

// Count returns the number of mirrors
func (ml *MirrorList) Count() int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return len(ml.mirrors)
}

// GetAll returns all mirrors
func (ml *MirrorList) GetAll() []*Mirror {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	result := make([]*Mirror, len(ml.mirrors))
	copy(result, ml.mirrors)
	return result
}

// Next returns the next mirror to try based on strategy
func (ml *MirrorList) Next() *Mirror {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if len(ml.mirrors) == 0 {
		return nil
	}

	// Filter healthy mirrors
	healthy := ml.getHealthyMirrors()
	if len(healthy) == 0 {
		// All mirrors failed, reset and try again
		ml.resetHealth()
		healthy = ml.mirrors
	}

	switch ml.strategy {
	case MirrorStrategyRandom:
		return healthy[rand.Intn(len(healthy))]

	case MirrorStrategyFastest:
		return ml.selectFastest(healthy)

	case MirrorStrategyRoundRobin:
		mirror := healthy[ml.current%len(healthy)]
		ml.current++
		return mirror

	default: // MirrorStrategyFailover
		// Return highest priority healthy mirror
		return ml.selectByPriority(healthy)
	}
}

// MarkFailed marks a mirror as failed
func (ml *MirrorList) MarkFailed(url string) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	for _, m := range ml.mirrors {
		if m.URL == url {
			m.FailCount++
			if m.FailCount >= 3 {
				m.Healthy = false
			}
			break
		}
	}
}

// MarkSuccess marks a mirror as successful
func (ml *MirrorList) MarkSuccess(url string, latency time.Duration) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	for _, m := range ml.mirrors {
		if m.URL == url {
			m.Healthy = true
			m.FailCount = 0
			m.LastLatency = latency
			m.LastChecked = time.Now()
			break
		}
	}
}

// HealthyCount returns the number of healthy mirrors
func (ml *MirrorList) HealthyCount() int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	count := 0
	for _, m := range ml.mirrors {
		if m.Healthy {
			count++
		}
	}
	return count
}

// getHealthyMirrors returns only healthy mirrors (must hold lock)
func (ml *MirrorList) getHealthyMirrors() []*Mirror {
	healthy := make([]*Mirror, 0, len(ml.mirrors))
	for _, m := range ml.mirrors {
		if m.Healthy {
			healthy = append(healthy, m)
		}
	}
	return healthy
}

// selectByPriority selects highest priority mirror
func (ml *MirrorList) selectByPriority(mirrors []*Mirror) *Mirror {
	if len(mirrors) == 0 {
		return nil
	}

	best := mirrors[0]
	for _, m := range mirrors[1:] {
		if m.Priority > best.Priority {
			best = m
		}
	}
	return best
}

// selectFastest selects the mirror with lowest latency
func (ml *MirrorList) selectFastest(mirrors []*Mirror) *Mirror {
	if len(mirrors) == 0 {
		return nil
	}

	var fastest *Mirror
	for _, m := range mirrors {
		if m.LastLatency > 0 {
			if fastest == nil || m.LastLatency < fastest.LastLatency {
				fastest = m
			}
		}
	}

	// If no latency data, return first mirror
	if fastest == nil {
		return mirrors[0]
	}
	return fastest
}

// resetHealth resets all mirrors to healthy
func (ml *MirrorList) resetHealth() {
	for _, m := range ml.mirrors {
		m.Healthy = true
		m.FailCount = 0
	}
}

// MirrorDownloader wraps a downloader with mirror support
type MirrorDownloader struct {
	downloader *Downloader
	mirrors    *MirrorList
	maxRetries int
}

// NewMirrorDownloader creates a mirror-aware downloader
func NewMirrorDownloader(downloader *Downloader, mirrors *MirrorList) *MirrorDownloader {
	return &MirrorDownloader{
		downloader: downloader,
		mirrors:    mirrors,
		maxRetries: 3,
	}
}

// SetMaxRetries sets the maximum retry count per mirror
func (md *MirrorDownloader) SetMaxRetries(n int) {
	md.maxRetries = n
}

// Download tries mirrors until one succeeds
func (md *MirrorDownloader) Download(ctx context.Context, outputPath string) error {
	if md.mirrors.Count() == 0 {
		return fmt.Errorf("no mirrors configured")
	}

	var lastErr error
	triedURLs := make(map[string]bool)

	// Try each mirror up to maxRetries times total
	for attempt := 0; attempt < md.mirrors.Count()*md.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		mirror := md.mirrors.Next()
		if mirror == nil {
			break
		}

		// Skip if we've tried this URL too many times
		if triedURLs[mirror.URL] {
			continue
		}

		startTime := time.Now()
		err := md.downloader.Download(ctx, mirror.URL, outputPath)
		latency := time.Since(startTime)

		if err == nil {
			md.mirrors.MarkSuccess(mirror.URL, latency)
			return nil
		}

		lastErr = err
		md.mirrors.MarkFailed(mirror.URL)
		triedURLs[mirror.URL] = true
	}

	if lastErr != nil {
		return fmt.Errorf("all mirrors failed, last error: %w", lastErr)
	}
	return fmt.Errorf("all mirrors exhausted")
}

// SetProgressCallback sets progress callback on underlying downloader
func (md *MirrorDownloader) SetProgressCallback(cb ProgressCallback) {
	md.downloader.SetProgressCallback(cb)
}

// GetProgress returns current progress
func (md *MirrorDownloader) GetProgress() Progress {
	return md.downloader.GetProgress()
}

// ParseMirrorFile parses a mirror file (one URL per line)
func ParseMirrorFile(content string) []string {
	var mirrors []string
	lines := splitLines(content)

	for _, line := range lines {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		mirrors = append(mirrors, line)
	}

	return mirrors
}

// Helper functions
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
