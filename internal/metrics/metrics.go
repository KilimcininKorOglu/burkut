// Package metrics provides Prometheus-compatible metrics for download operations.
package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics holds all download metrics
type Metrics struct {
	// Counters
	downloadsTotal       atomic.Int64 // Total number of downloads started
	downloadsCompleted   atomic.Int64 // Successfully completed downloads
	downloadsFailed      atomic.Int64 // Failed downloads
	bytesDownloadedTotal atomic.Int64 // Total bytes downloaded

	// Gauges
	activeDownloads   atomic.Int64 // Currently active downloads
	activeConnections atomic.Int64 // Currently active connections
	currentSpeed      atomic.Int64 // Current download speed (bytes/sec)

	// Histogram buckets for download duration
	durationBuckets map[string]int64 // bucket label -> count

	// Start time for uptime calculation
	startTime time.Time

	mu sync.RWMutex
}

// New creates a new Metrics instance
func New() *Metrics {
	return &Metrics{
		startTime: time.Now(),
		durationBuckets: map[string]int64{
			"le_1s":   0,
			"le_5s":   0,
			"le_30s":  0,
			"le_60s":  0,
			"le_300s": 0,
			"le_inf":  0,
		},
	}
}

// IncDownloadsTotal increments the total downloads counter
func (m *Metrics) IncDownloadsTotal() {
	m.downloadsTotal.Add(1)
}

// IncDownloadsCompleted increments the completed downloads counter
func (m *Metrics) IncDownloadsCompleted() {
	m.downloadsCompleted.Add(1)
}

// IncDownloadsFailed increments the failed downloads counter
func (m *Metrics) IncDownloadsFailed() {
	m.downloadsFailed.Add(1)
}

// AddBytesDownloaded adds to the total bytes downloaded
func (m *Metrics) AddBytesDownloaded(bytes int64) {
	m.bytesDownloadedTotal.Add(bytes)
}

// SetActiveDownloads sets the number of active downloads
func (m *Metrics) SetActiveDownloads(count int64) {
	m.activeDownloads.Store(count)
}

// IncActiveDownloads increments active downloads
func (m *Metrics) IncActiveDownloads() {
	m.activeDownloads.Add(1)
}

// DecActiveDownloads decrements active downloads
func (m *Metrics) DecActiveDownloads() {
	m.activeDownloads.Add(-1)
}

// SetActiveConnections sets the number of active connections
func (m *Metrics) SetActiveConnections(count int64) {
	m.activeConnections.Store(count)
}

// SetCurrentSpeed sets the current download speed
func (m *Metrics) SetCurrentSpeed(bytesPerSec int64) {
	m.currentSpeed.Store(bytesPerSec)
}

// RecordDownloadDuration records a download duration in the histogram
func (m *Metrics) RecordDownloadDuration(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	secs := d.Seconds()
	switch {
	case secs <= 1:
		m.durationBuckets["le_1s"]++
	case secs <= 5:
		m.durationBuckets["le_5s"]++
	case secs <= 30:
		m.durationBuckets["le_30s"]++
	case secs <= 60:
		m.durationBuckets["le_60s"]++
	case secs <= 300:
		m.durationBuckets["le_300s"]++
	default:
		m.durationBuckets["le_inf"]++
	}
}

// GetStats returns current metrics as a map
func (m *Metrics) GetStats() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]int64{
		"downloads_total":        m.downloadsTotal.Load(),
		"downloads_completed":    m.downloadsCompleted.Load(),
		"downloads_failed":       m.downloadsFailed.Load(),
		"bytes_downloaded_total": m.bytesDownloadedTotal.Load(),
		"active_downloads":       m.activeDownloads.Load(),
		"active_connections":     m.activeConnections.Load(),
		"current_speed":          m.currentSpeed.Load(),
		"uptime_seconds":         int64(time.Since(m.startTime).Seconds()),
	}

	// Add histogram buckets
	for k, v := range m.durationBuckets {
		stats["download_duration_seconds_bucket_"+k] = v
	}

	return stats
}

// Handler returns an HTTP handler for Prometheus metrics endpoint
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		// Get current stats
		stats := m.GetStats()

		// Write Prometheus format metrics
		_, _ = fmt.Fprintln(w, "# HELP burkut_downloads_total Total number of downloads started")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_downloads_total counter")
		_, _ = fmt.Fprintf(w, "burkut_downloads_total %d\n", stats["downloads_total"])

		_, _ = fmt.Fprintln(w, "# HELP burkut_downloads_completed_total Successfully completed downloads")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_downloads_completed_total counter")
		_, _ = fmt.Fprintf(w, "burkut_downloads_completed_total %d\n", stats["downloads_completed"])

		_, _ = fmt.Fprintln(w, "# HELP burkut_downloads_failed_total Failed downloads")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_downloads_failed_total counter")
		_, _ = fmt.Fprintf(w, "burkut_downloads_failed_total %d\n", stats["downloads_failed"])

		_, _ = fmt.Fprintln(w, "# HELP burkut_bytes_downloaded_total Total bytes downloaded")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_bytes_downloaded_total counter")
		_, _ = fmt.Fprintf(w, "burkut_bytes_downloaded_total %d\n", stats["bytes_downloaded_total"])

		_, _ = fmt.Fprintln(w, "# HELP burkut_active_downloads Currently active downloads")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_active_downloads gauge")
		_, _ = fmt.Fprintf(w, "burkut_active_downloads %d\n", stats["active_downloads"])

		_, _ = fmt.Fprintln(w, "# HELP burkut_active_connections Currently active connections")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_active_connections gauge")
		_, _ = fmt.Fprintf(w, "burkut_active_connections %d\n", stats["active_connections"])

		_, _ = fmt.Fprintln(w, "# HELP burkut_download_speed_bytes Current download speed in bytes per second")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_download_speed_bytes gauge")
		_, _ = fmt.Fprintf(w, "burkut_download_speed_bytes %d\n", stats["current_speed"])

		_, _ = fmt.Fprintln(w, "# HELP burkut_uptime_seconds Time since start in seconds")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_uptime_seconds counter")
		_, _ = fmt.Fprintf(w, "burkut_uptime_seconds %d\n", stats["uptime_seconds"])

		// Duration histogram
		_, _ = fmt.Fprintln(w, "# HELP burkut_download_duration_seconds_bucket Download duration histogram")
		_, _ = fmt.Fprintln(w, "# TYPE burkut_download_duration_seconds_bucket histogram")
		_, _ = fmt.Fprintf(w, "burkut_download_duration_seconds_bucket{le=\"1\"} %d\n", stats["download_duration_seconds_bucket_le_1s"])
		_, _ = fmt.Fprintf(w, "burkut_download_duration_seconds_bucket{le=\"5\"} %d\n", stats["download_duration_seconds_bucket_le_5s"])
		_, _ = fmt.Fprintf(w, "burkut_download_duration_seconds_bucket{le=\"30\"} %d\n", stats["download_duration_seconds_bucket_le_30s"])
		_, _ = fmt.Fprintf(w, "burkut_download_duration_seconds_bucket{le=\"60\"} %d\n", stats["download_duration_seconds_bucket_le_60s"])
		_, _ = fmt.Fprintf(w, "burkut_download_duration_seconds_bucket{le=\"300\"} %d\n", stats["download_duration_seconds_bucket_le_300s"])
		_, _ = fmt.Fprintf(w, "burkut_download_duration_seconds_bucket{le=\"+Inf\"} %d\n", stats["download_duration_seconds_bucket_le_inf"])
	})
}

// Server wraps an HTTP server for metrics
type Server struct {
	server  *http.Server
	metrics *Metrics
}

// NewServer creates a new metrics server
func NewServer(addr string, m *Metrics) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	return &Server{
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		metrics: m,
	}
}

// Start starts the metrics server in a goroutine
func (s *Server) Start() error {
	go func() {
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			// Log error but don't crash
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()
	return nil
}

// Stop stops the metrics server
func (s *Server) Stop() error {
	return s.server.Close()
}

// Addr returns the server address
func (s *Server) Addr() string {
	return s.server.Addr
}
