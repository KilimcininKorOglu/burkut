package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetrics_Counters(t *testing.T) {
	m := New()

	// Test initial values
	stats := m.GetStats()
	if stats["downloads_total"] != 0 {
		t.Errorf("Initial downloads_total = %d, want 0", stats["downloads_total"])
	}

	// Test incrementing
	m.IncDownloadsTotal()
	m.IncDownloadsTotal()
	m.IncDownloadsCompleted()
	m.IncDownloadsFailed()
	m.AddBytesDownloaded(1024)
	m.AddBytesDownloaded(2048)

	stats = m.GetStats()
	if stats["downloads_total"] != 2 {
		t.Errorf("downloads_total = %d, want 2", stats["downloads_total"])
	}
	if stats["downloads_completed"] != 1 {
		t.Errorf("downloads_completed = %d, want 1", stats["downloads_completed"])
	}
	if stats["downloads_failed"] != 1 {
		t.Errorf("downloads_failed = %d, want 1", stats["downloads_failed"])
	}
	if stats["bytes_downloaded_total"] != 3072 {
		t.Errorf("bytes_downloaded_total = %d, want 3072", stats["bytes_downloaded_total"])
	}
}

func TestMetrics_Gauges(t *testing.T) {
	m := New()

	m.SetActiveDownloads(5)
	m.SetActiveConnections(10)
	m.SetCurrentSpeed(1024 * 1024) // 1 MB/s

	stats := m.GetStats()
	if stats["active_downloads"] != 5 {
		t.Errorf("active_downloads = %d, want 5", stats["active_downloads"])
	}
	if stats["active_connections"] != 10 {
		t.Errorf("active_connections = %d, want 10", stats["active_connections"])
	}
	if stats["current_speed"] != 1024*1024 {
		t.Errorf("current_speed = %d, want %d", stats["current_speed"], 1024*1024)
	}

	// Test inc/dec
	m.IncActiveDownloads()
	m.IncActiveDownloads()
	m.DecActiveDownloads()

	stats = m.GetStats()
	if stats["active_downloads"] != 6 {
		t.Errorf("active_downloads after inc/dec = %d, want 6", stats["active_downloads"])
	}
}

func TestMetrics_DurationHistogram(t *testing.T) {
	m := New()

	// Record various durations
	m.RecordDownloadDuration(500 * time.Millisecond) // <= 1s
	m.RecordDownloadDuration(3 * time.Second)        // <= 5s
	m.RecordDownloadDuration(20 * time.Second)       // <= 30s
	m.RecordDownloadDuration(45 * time.Second)       // <= 60s
	m.RecordDownloadDuration(120 * time.Second)      // <= 300s
	m.RecordDownloadDuration(600 * time.Second)      // > 300s (inf)

	stats := m.GetStats()
	if stats["download_duration_seconds_bucket_le_1s"] != 1 {
		t.Errorf("le_1s bucket = %d, want 1", stats["download_duration_seconds_bucket_le_1s"])
	}
	if stats["download_duration_seconds_bucket_le_5s"] != 1 {
		t.Errorf("le_5s bucket = %d, want 1", stats["download_duration_seconds_bucket_le_5s"])
	}
	if stats["download_duration_seconds_bucket_le_inf"] != 1 {
		t.Errorf("le_inf bucket = %d, want 1", stats["download_duration_seconds_bucket_le_inf"])
	}
}

func TestMetrics_Handler(t *testing.T) {
	m := New()

	// Set some metrics
	m.IncDownloadsTotal()
	m.IncDownloadsCompleted()
	m.AddBytesDownloaded(1024)
	m.SetActiveDownloads(2)

	// Create test server
	handler := m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Check for expected metrics
	expectedMetrics := []string{
		"burkut_downloads_total 1",
		"burkut_downloads_completed_total 1",
		"burkut_bytes_downloaded_total 1024",
		"burkut_active_downloads 2",
		"# TYPE burkut_downloads_total counter",
		"# TYPE burkut_active_downloads gauge",
	}

	for _, expected := range expectedMetrics {
		if !strings.Contains(bodyStr, expected) {
			t.Errorf("Response missing: %s", expected)
		}
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Content-Type = %s, want text/plain", contentType)
	}
}

func TestMetrics_Uptime(t *testing.T) {
	m := New()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	stats := m.GetStats()
	if stats["uptime_seconds"] < 0 {
		t.Errorf("uptime_seconds = %d, should be >= 0", stats["uptime_seconds"])
	}
}
