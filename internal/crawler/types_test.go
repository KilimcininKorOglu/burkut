package crawler

import (
	"net/url"
	"testing"
)

func TestNewURLQueue(t *testing.T) {
	q := NewURLQueue()
	if q == nil {
		t.Fatal("NewURLQueue returned nil")
	}
	if q.Count() != 0 {
		t.Errorf("New queue should be empty, got %d items", q.Count())
	}
}

func TestURLQueue_Add(t *testing.T) {
	q := NewURLQueue()

	u, _ := url.Parse("https://example.com/page1")
	item := &CrawlItem{URL: u, Depth: 0}

	// First add should succeed
	if !q.Add(item) {
		t.Error("First add should return true")
	}
	if q.Count() != 1 {
		t.Errorf("Count = %d, want 1", q.Count())
	}

	// Duplicate add should fail
	if q.Add(item) {
		t.Error("Duplicate add should return false")
	}
	if q.Count() != 1 {
		t.Errorf("Count after duplicate = %d, want 1", q.Count())
	}
}

func TestURLQueue_Next(t *testing.T) {
	q := NewURLQueue()

	u1, _ := url.Parse("https://example.com/page1")
	u2, _ := url.Parse("https://example.com/page2")

	q.Add(&CrawlItem{URL: u1, Depth: 0})
	q.Add(&CrawlItem{URL: u2, Depth: 1})

	// Get first item
	item := q.Next()
	if item == nil {
		t.Fatal("Next returned nil")
	}
	if item.URL.String() != u1.String() {
		t.Errorf("URL = %s, want %s", item.URL, u1)
	}
	if item.Status != StatusInProgress {
		t.Errorf("Status = %v, want InProgress", item.Status)
	}

	// Get second item
	item = q.Next()
	if item == nil {
		t.Fatal("Second Next returned nil")
	}
	if item.URL.String() != u2.String() {
		t.Errorf("URL = %s, want %s", item.URL, u2)
	}

	// No more pending items
	item = q.Next()
	if item != nil {
		t.Error("Expected nil when no pending items")
	}
}

func TestURLQueue_IsVisited(t *testing.T) {
	q := NewURLQueue()

	u1, _ := url.Parse("https://example.com/page1")
	u2, _ := url.Parse("https://example.com/page2")

	q.Add(&CrawlItem{URL: u1})

	if !q.IsVisited(u1) {
		t.Error("u1 should be visited")
	}
	if q.IsVisited(u2) {
		t.Error("u2 should not be visited")
	}
}

func TestURLQueue_MarkVisited(t *testing.T) {
	q := NewURLQueue()

	u, _ := url.Parse("https://example.com/page1")

	q.MarkVisited(u)

	if !q.IsVisited(u) {
		t.Error("URL should be marked as visited")
	}
	// But not in queue
	if q.Count() != 0 {
		t.Errorf("Count = %d, want 0", q.Count())
	}
}

func TestURLQueue_Counts(t *testing.T) {
	q := NewURLQueue()

	u1, _ := url.Parse("https://example.com/page1")
	u2, _ := url.Parse("https://example.com/page2")
	u3, _ := url.Parse("https://example.com/page3")

	q.Add(&CrawlItem{URL: u1, Status: StatusPending})
	q.Add(&CrawlItem{URL: u2, Status: StatusPending})
	q.Add(&CrawlItem{URL: u3, Status: StatusPending})

	// Get one item (changes to InProgress)
	item := q.Next()
	item.Status = StatusCompleted

	// Get another
	item = q.Next()
	item.Status = StatusFailed

	if q.PendingCount() != 1 {
		t.Errorf("PendingCount = %d, want 1", q.PendingCount())
	}
	if q.CompletedCount() != 1 {
		t.Errorf("CompletedCount = %d, want 1", q.CompletedCount())
	}
	if q.FailedCount() != 1 {
		t.Errorf("FailedCount = %d, want 1", q.FailedCount())
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/page", "https://example.com/page"},
		{"https://example.com/page#section", "https://example.com/page"},
		{"http://example.com:80/page", "http://example.com/page"},
		{"https://example.com:443/page", "https://example.com/page"},
		{"https://EXAMPLE.COM/Page", "https://EXAMPLE.COM/Page"}, // Path case preserved
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			u, _ := url.Parse(tt.input)
			result := normalizeURL(u)
			if result != tt.expected {
				t.Errorf("normalizeURL(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCrawlStatus_String(t *testing.T) {
	tests := []struct {
		status   CrawlStatus
		expected string
	}{
		{StatusPending, "pending"},
		{StatusInProgress, "in_progress"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusSkipped, "skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.status.String() != tt.expected {
				t.Errorf("String() = %s, want %s", tt.status.String(), tt.expected)
			}
		})
	}
}
