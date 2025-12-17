// Package download provides download management functionality.
package download

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// QueueItem represents a single download in the queue
type QueueItem struct {
	ID          int
	URL         string
	OutputPath  string
	Checksum    string // Optional checksum for verification
	Status      QueueStatus
	Error       error
	StartTime   time.Time
	EndTime     time.Time
	Downloaded  int64
	TotalSize   int64
	Retries     int
}

// QueueStatus represents the status of a queue item
type QueueStatus int

const (
	QueueStatusPending QueueStatus = iota
	QueueStatusDownloading
	QueueStatusCompleted
	QueueStatusFailed
	QueueStatusSkipped
	QueueStatusCanceled
)

func (s QueueStatus) String() string {
	switch s {
	case QueueStatusPending:
		return "pending"
	case QueueStatusDownloading:
		return "downloading"
	case QueueStatusCompleted:
		return "completed"
	case QueueStatusFailed:
		return "failed"
	case QueueStatusSkipped:
		return "skipped"
	case QueueStatusCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// QueueStats holds queue statistics
type QueueStats struct {
	Total       int
	Pending     int
	Downloading int
	Completed   int
	Failed      int
	Skipped     int
	TotalBytes  int64
	Downloaded  int64
}

// Queue manages a list of downloads
type Queue struct {
	items      []*QueueItem
	outputDir  string
	mu         sync.RWMutex
}

// NewQueue creates a new download queue
func NewQueue(outputDir string) *Queue {
	return &Queue{
		items:     make([]*QueueItem, 0),
		outputDir: outputDir,
	}
}

// Add adds a URL to the queue
func (q *Queue) Add(rawURL string) error {
	return q.AddWithOptions(rawURL, "", "")
}

// AddWithOptions adds a URL with optional output path and checksum
func (q *Queue) AddWithOptions(rawURL, outputPath, checksum string) error {
	// Validate URL
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("empty URL")
	}

	// Skip comments and empty lines
	if strings.HasPrefix(rawURL, "#") {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid URL: missing scheme or host")
	}

	// Determine output path if not specified
	if outputPath == "" {
		outputPath = extractFilename(rawURL)
		if q.outputDir != "" {
			outputPath = filepath.Join(q.outputDir, outputPath)
		}
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	item := &QueueItem{
		ID:         len(q.items),
		URL:        rawURL,
		OutputPath: outputPath,
		Checksum:   checksum,
		Status:     QueueStatusPending,
	}

	q.items = append(q.items, item)
	return nil
}

// LoadFromFile loads URLs from a file (one URL per line)
func (q *Queue) LoadFromFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line: URL [OUTPUT] [CHECKSUM]
		// Formats supported:
		// - URL
		// - URL OUTPUT
		// - URL OUTPUT CHECKSUM
		// - URL|OUTPUT|CHECKSUM (pipe separated)
		
		var rawURL, outputPath, checksum string

		if strings.Contains(line, "|") {
			// Pipe-separated format
			parts := strings.Split(line, "|")
			rawURL = strings.TrimSpace(parts[0])
			if len(parts) > 1 {
				outputPath = strings.TrimSpace(parts[1])
			}
			if len(parts) > 2 {
				checksum = strings.TrimSpace(parts[2])
			}
		} else {
			// Space-separated format
			parts := strings.Fields(line)
			rawURL = parts[0]
			if len(parts) > 1 {
				outputPath = parts[1]
			}
			if len(parts) > 2 {
				checksum = parts[2]
			}
		}

		if err := q.AddWithOptions(rawURL, outputPath, checksum); err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	return nil
}

// Items returns all queue items
func (q *Queue) Items() []*QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	items := make([]*QueueItem, len(q.items))
	copy(items, q.items)
	return items
}

// Get returns a queue item by ID
func (q *Queue) Get(id int) *QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if id < 0 || id >= len(q.items) {
		return nil
	}
	return q.items[id]
}

// UpdateStatus updates the status of a queue item
func (q *Queue) UpdateStatus(id int, status QueueStatus) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if id >= 0 && id < len(q.items) {
		q.items[id].Status = status
		if status == QueueStatusDownloading {
			q.items[id].StartTime = time.Now()
		} else if status == QueueStatusCompleted || status == QueueStatusFailed {
			q.items[id].EndTime = time.Now()
		}
	}
}

// UpdateProgress updates the progress of a queue item
func (q *Queue) UpdateProgress(id int, downloaded, total int64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if id >= 0 && id < len(q.items) {
		q.items[id].Downloaded = downloaded
		q.items[id].TotalSize = total
	}
}

// SetError sets the error for a queue item
func (q *Queue) SetError(id int, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if id >= 0 && id < len(q.items) {
		q.items[id].Error = err
		q.items[id].Status = QueueStatusFailed
		q.items[id].EndTime = time.Now()
	}
}

// NextPending returns the next pending item
func (q *Queue) NextPending() *QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, item := range q.items {
		if item.Status == QueueStatusPending {
			return item
		}
	}
	return nil
}

// Stats returns queue statistics
func (q *Queue) Stats() QueueStats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := QueueStats{
		Total: len(q.items),
	}

	for _, item := range q.items {
		switch item.Status {
		case QueueStatusPending:
			stats.Pending++
		case QueueStatusDownloading:
			stats.Downloading++
		case QueueStatusCompleted:
			stats.Completed++
		case QueueStatusFailed:
			stats.Failed++
		case QueueStatusSkipped:
			stats.Skipped++
		}
		stats.TotalBytes += item.TotalSize
		stats.Downloaded += item.Downloaded
	}

	return stats
}

// IsComplete returns true if all items are processed
func (q *Queue) IsComplete() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, item := range q.items {
		if item.Status == QueueStatusPending || item.Status == QueueStatusDownloading {
			return false
		}
	}
	return true
}

// Count returns the number of items in the queue
func (q *Queue) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}

// Clear removes all items from the queue
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = make([]*QueueItem, 0)
}

// extractFilename extracts filename from URL
func extractFilename(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "download"
	}

	path := parsed.Path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[idx+1:]
	}

	if decoded, err := url.QueryUnescape(path); err == nil {
		path = decoded
	}

	if path == "" {
		return "download"
	}

	return path
}

// QueueCallback is called for each download event
type QueueCallback func(item *QueueItem, event QueueEvent)

// QueueEvent represents a queue event type
type QueueEvent int

const (
	QueueEventStarted QueueEvent = iota
	QueueEventProgress
	QueueEventCompleted
	QueueEventFailed
	QueueEventSkipped
)

// QueueManager manages parallel queue processing
type QueueManager struct {
	queue       *Queue
	concurrency int
	callback    QueueCallback
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewQueueManager creates a new queue manager
func NewQueueManager(queue *Queue, concurrency int) *QueueManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &QueueManager{
		queue:       queue,
		concurrency: concurrency,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// SetCallback sets the event callback
func (qm *QueueManager) SetCallback(cb QueueCallback) {
	qm.callback = cb
}

// Stop stops the queue processing
func (qm *QueueManager) Stop() {
	qm.cancel()
	qm.wg.Wait()
}

// Queue returns the underlying queue
func (qm *QueueManager) Queue() *Queue {
	return qm.queue
}

// Context returns the queue manager context
func (qm *QueueManager) Context() context.Context {
	return qm.ctx
}
