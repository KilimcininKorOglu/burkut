package crawler

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config holds crawler configuration
type Config struct {
	// Maximum depth to crawl (0 = only start URL)
	MaxDepth int

	// Wait time between requests
	WaitTime time.Duration

	// Random wait variation (0 to this value added to WaitTime)
	RandomWait time.Duration

	// Respect robots.txt
	RespectRobots bool

	// User agent string
	UserAgent string

	// Output directory
	OutputDir string

	// Convert links to local paths
	ConvertLinks bool

	// Number of parallel workers
	Workers int

	// Filter configuration
	Filter *Filter

	// Timeout for each request
	Timeout time.Duration

	// Maximum file size to download (0 = unlimited)
	MaxFileSize int64

	// HTTP client to use
	HTTPClient *http.Client
}

// DefaultConfig returns default crawler configuration
func DefaultConfig() *Config {
	return &Config{
		MaxDepth:      5,
		WaitTime:      time.Second,
		RandomWait:    500 * time.Millisecond,
		RespectRobots: true,
		UserAgent:     "Burkut/1.0 (Website Mirror)",
		OutputDir:     ".",
		ConvertLinks:  false,
		Workers:       2,
		Timeout:       30 * time.Second,
		MaxFileSize:   0,
	}
}

// Stats holds crawl statistics
type Stats struct {
	StartTime       time.Time
	EndTime         time.Time
	TotalURLs       int
	DownloadedURLs  int
	SkippedURLs     int
	FailedURLs      int
	TotalBytes      int64
	CurrentURL      string
	CurrentDepth    int
	mu              sync.RWMutex
}

// Crawler handles recursive website downloading
type Crawler struct {
	config  *Config
	queue   *URLQueue
	robots  *RobotsChecker
	stats   *Stats
	baseURL *url.URL

	// Progress callback
	progressFn func(*Stats)

	// Error callback
	errorFn func(string, error)

	// HTTP client
	httpClient *http.Client

	// Worker synchronization
	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// NewCrawler creates a new crawler
func NewCrawler(config *Config) *Crawler {
	if config == nil {
		config = DefaultConfig()
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	return &Crawler{
		config:     config,
		queue:      NewURLQueue(),
		robots:     NewRobotsChecker(config.UserAgent, config.RespectRobots),
		stats:      &Stats{},
		httpClient: httpClient,
	}
}

// SetProgressCallback sets the progress callback function
func (c *Crawler) SetProgressCallback(fn func(*Stats)) {
	c.progressFn = fn
}

// SetErrorCallback sets the error callback function
func (c *Crawler) SetErrorCallback(fn func(string, error)) {
	c.errorFn = fn
}

// Crawl starts crawling from the given URL
func (c *Crawler) Crawl(ctx context.Context, startURL string) error {
	// Parse start URL
	u, err := url.Parse(startURL)
	if err != nil {
		return fmt.Errorf("invalid start URL: %w", err)
	}

	c.baseURL = u

	// Setup filter if not provided
	if c.config.Filter == nil {
		c.config.Filter = NewFilter(u)
	} else {
		c.config.Filter.BaseURL = u
	}

	// Create output directory
	if err := os.MkdirAll(c.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Create cancellable context
	ctx, c.cancel = context.WithCancel(ctx)

	// Initialize stats
	c.stats.StartTime = time.Now()

	// Add start URL to queue
	c.queue.Add(&CrawlItem{
		URL:     u,
		Depth:   0,
		FoundAt: time.Now(),
		Status:  StatusPending,
	})

	// Start workers
	workCh := make(chan *CrawlItem, c.config.Workers*2)

	for i := 0; i < c.config.Workers; i++ {
		c.wg.Add(1)
		go c.worker(ctx, workCh)
	}

	// Feed work to workers
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(workCh)
				return
			default:
				item := c.queue.Next()
				if item == nil {
					// No more work, check if workers are done
					time.Sleep(100 * time.Millisecond)
					if c.queue.PendingCount() == 0 {
						close(workCh)
						return
					}
					continue
				}
				workCh <- item
			}
		}
	}()

	// Wait for workers to finish
	c.wg.Wait()

	c.stats.EndTime = time.Now()

	// Convert links if enabled
	if c.config.ConvertLinks {
		ConvertAllFiles(c.queue, c.config.OutputDir, c.baseURL)
	}

	return nil
}

// worker processes URLs from the work channel
func (c *Crawler) worker(ctx context.Context, workCh <-chan *CrawlItem) {
	defer c.wg.Done()

	for item := range workCh {
		select {
		case <-ctx.Done():
			item.Status = StatusSkipped
			return
		default:
		}

		c.processItem(ctx, item)

		// Apply wait time
		c.wait(ctx)
	}
}

// processItem downloads and processes a single URL
func (c *Crawler) processItem(ctx context.Context, item *CrawlItem) {
	// Update stats
	c.stats.mu.Lock()
	c.stats.CurrentURL = item.URL.String()
	c.stats.CurrentDepth = item.Depth
	c.stats.mu.Unlock()

	// Check robots.txt
	allowed, err := c.robots.IsAllowed(ctx, item.URL)
	if err == nil && !allowed {
		item.Status = StatusSkipped
		c.stats.mu.Lock()
		c.stats.SkippedURLs++
		c.stats.mu.Unlock()
		return
	}

	// Check crawl delay from robots.txt
	if delay := c.robots.GetCrawlDelay(ctx, item.URL); delay > 0 {
		time.Sleep(delay)
	}

	// Download the URL
	localPath, contentType, body, err := c.download(ctx, item)
	if err != nil {
		item.Status = StatusFailed
		item.Error = err
		c.stats.mu.Lock()
		c.stats.FailedURLs++
		c.stats.mu.Unlock()
		if c.errorFn != nil {
			c.errorFn(item.URL.String(), err)
		}
		return
	}

	item.LocalPath = localPath
	item.Status = StatusCompleted

	c.stats.mu.Lock()
	c.stats.DownloadedURLs++
	c.stats.TotalURLs++
	c.stats.mu.Unlock()

	// Report progress
	if c.progressFn != nil {
		c.progressFn(c.GetStats())
	}

	// Extract and queue links if not at max depth
	if item.Depth < c.config.MaxDepth {
		c.extractLinks(ctx, item, contentType, body)
	}
}

// download downloads a URL and saves it locally
func (c *Crawler) download(ctx context.Context, item *CrawlItem) (string, string, []byte, error) {
	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL.String(), nil)
	if err != nil {
		return "", "", nil, err
	}

	req.Header.Set("User-Agent", c.config.UserAgent)
	if item.Referrer != "" {
		req.Header.Set("Referer", item.Referrer)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Check file size
	if c.config.MaxFileSize > 0 && resp.ContentLength > c.config.MaxFileSize {
		return "", "", nil, fmt.Errorf("file too large: %d bytes", resp.ContentLength)
	}

	// Read body
	var body []byte
	if c.config.MaxFileSize > 0 {
		body, err = io.ReadAll(io.LimitReader(resp.Body, c.config.MaxFileSize+1))
		if int64(len(body)) > c.config.MaxFileSize {
			return "", "", nil, fmt.Errorf("file too large")
		}
	} else {
		body, err = io.ReadAll(resp.Body)
	}
	if err != nil {
		return "", "", nil, err
	}

	// Determine local path
	localPath := c.urlToLocalPath(item.URL)

	// Create directory
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", nil, err
	}

	// Write file
	if err := os.WriteFile(localPath, body, 0644); err != nil {
		return "", "", nil, err
	}

	// Update stats
	c.stats.mu.Lock()
	c.stats.TotalBytes += int64(len(body))
	c.stats.mu.Unlock()

	contentType := resp.Header.Get("Content-Type")
	return localPath, contentType, body, nil
}

// extractLinks extracts links from content and adds them to queue
func (c *Crawler) extractLinks(ctx context.Context, item *CrawlItem, contentType string, body []byte) {
	var links []*ExtractedLink

	// Parse HTML
	if IsHTMLContentType(contentType) {
		var err error
		links, err = ParseHTML(strings.NewReader(string(body)), item.URL)
		if err != nil {
			return
		}
	} else if IsCSSContentType(contentType) {
		links = ParseCSS(string(body), item.URL)
	}

	// Add links to queue
	for _, link := range links {
		// Check filter
		if !c.config.Filter.ShouldCrawl(link.Resolved, link.Type) {
			continue
		}

		// Check if we should follow links from this type
		depth := item.Depth + 1
		if !c.config.Filter.ShouldFollowLinks(link.Resolved, link.Type) {
			// Still download requisites but don't increase depth
			if !isPageRequisite(link.Type) {
				continue
			}
		}

		// Add to queue
		c.queue.Add(&CrawlItem{
			URL:      link.Resolved,
			Depth:    depth,
			Parent:   item.URL.String(),
			Referrer: item.URL.String(),
			FoundAt:  time.Now(),
			Status:   StatusPending,
		})
	}
}

// urlToLocalPath converts a URL to a local file path
func (c *Crawler) urlToLocalPath(u *url.URL) string {
	// Start with host directory
	path := u.Hostname()

	// Add URL path
	urlPath := u.Path
	if urlPath == "" || urlPath == "/" {
		urlPath = "/index.html"
	}

	// Handle directory paths (add index.html)
	if strings.HasSuffix(urlPath, "/") {
		urlPath += "index.html"
	}

	// Check if path has extension
	if filepath.Ext(urlPath) == "" {
		urlPath += ".html"
	}

	path = filepath.Join(path, urlPath)

	// Add query string to filename if present
	if u.RawQuery != "" {
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)

		// Hash query string for filename
		queryHash := hashString(u.RawQuery)
		path = filepath.Join(dir, name+"_"+queryHash[:8]+ext)
	}

	// Clean path
	path = filepath.Clean(path)

	// Ensure it's under output dir
	return filepath.Join(c.config.OutputDir, path)
}

// hashString creates a simple hash of a string
func hashString(s string) string {
	h := uint64(0)
	for _, c := range s {
		h = h*31 + uint64(c)
	}
	return fmt.Sprintf("%016x", h)
}

// wait applies configured wait time between requests
func (c *Crawler) wait(ctx context.Context) {
	wait := c.config.WaitTime
	if c.config.RandomWait > 0 {
		wait += time.Duration(rand.Int63n(int64(c.config.RandomWait)))
	}

	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

// Stop stops the crawler
func (c *Crawler) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// GetStats returns current crawl statistics
func (c *Crawler) GetStats() *Stats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()

	return &Stats{
		StartTime:      c.stats.StartTime,
		EndTime:        c.stats.EndTime,
		TotalURLs:      c.stats.TotalURLs,
		DownloadedURLs: c.stats.DownloadedURLs,
		SkippedURLs:    c.stats.SkippedURLs,
		FailedURLs:     c.stats.FailedURLs,
		TotalBytes:     c.stats.TotalBytes,
		CurrentURL:     c.stats.CurrentURL,
		CurrentDepth:   c.stats.CurrentDepth,
	}
}

// GetQueue returns the URL queue (for inspection)
func (c *Crawler) GetQueue() *URLQueue {
	return c.queue
}
