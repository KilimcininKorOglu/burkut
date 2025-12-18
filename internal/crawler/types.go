// Package crawler provides recursive website download functionality.
package crawler

import (
	"net/url"
	"sync"
	"time"
)

// CrawlItem represents a URL to be crawled
type CrawlItem struct {
	URL       *url.URL
	Depth     int
	Parent    string // Parent URL that linked to this
	Referrer  string // HTTP Referer header value
	FoundAt   time.Time
	Status    CrawlStatus
	Error     error
	LocalPath string // Local file path after download
}

// CrawlStatus represents the status of a crawl item
type CrawlStatus int

const (
	StatusPending CrawlStatus = iota
	StatusInProgress
	StatusCompleted
	StatusFailed
	StatusSkipped
)

// String returns string representation of status
func (s CrawlStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusInProgress:
		return "in_progress"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	default:
		return "unknown"
	}
}

// URLQueue is a thread-safe queue for URLs to crawl
type URLQueue struct {
	items   []*CrawlItem
	visited map[string]bool
	mu      sync.RWMutex
}

// NewURLQueue creates a new URL queue
func NewURLQueue() *URLQueue {
	return &URLQueue{
		items:   make([]*CrawlItem, 0),
		visited: make(map[string]bool),
	}
}

// Add adds a URL to the queue if not already visited
// Returns true if added, false if already visited
func (q *URLQueue) Add(item *CrawlItem) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := normalizeURL(item.URL)
	if q.visited[key] {
		return false
	}

	q.visited[key] = true
	q.items = append(q.items, item)
	return true
}

// Next returns the next pending item, or nil if queue is empty
func (q *URLQueue) Next() *CrawlItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		if item.Status == StatusPending {
			item.Status = StatusInProgress
			return item
		}
	}
	return nil
}

// IsVisited checks if a URL has been visited
func (q *URLQueue) IsVisited(u *url.URL) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.visited[normalizeURL(u)]
}

// MarkVisited marks a URL as visited without adding to queue
func (q *URLQueue) MarkVisited(u *url.URL) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.visited[normalizeURL(u)] = true
}

// Count returns total items in queue
func (q *URLQueue) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}

// PendingCount returns number of pending items
func (q *URLQueue) PendingCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	count := 0
	for _, item := range q.items {
		if item.Status == StatusPending {
			count++
		}
	}
	return count
}

// CompletedCount returns number of completed items
func (q *URLQueue) CompletedCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	count := 0
	for _, item := range q.items {
		if item.Status == StatusCompleted {
			count++
		}
	}
	return count
}

// FailedCount returns number of failed items
func (q *URLQueue) FailedCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	count := 0
	for _, item := range q.items {
		if item.Status == StatusFailed {
			count++
		}
	}
	return count
}

// Items returns all items (for reporting)
func (q *URLQueue) Items() []*CrawlItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*CrawlItem, len(q.items))
	copy(result, q.items)
	return result
}

// normalizeURL creates a normalized string key for URL comparison
func normalizeURL(u *url.URL) string {
	// Normalize: lowercase scheme and host, remove default ports, remove fragments
	normalized := *u
	normalized.Fragment = ""
	normalized.RawFragment = ""

	// Remove default ports
	if (normalized.Scheme == "http" && normalized.Port() == "80") ||
		(normalized.Scheme == "https" && normalized.Port() == "443") {
		normalized.Host = normalized.Hostname()
	}

	return normalized.String()
}
