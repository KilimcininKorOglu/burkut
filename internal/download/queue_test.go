package download

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewQueue(t *testing.T) {
	q := NewQueue("/tmp/downloads")
	
	if q == nil {
		t.Fatal("NewQueue returned nil")
	}
	
	if q.Count() != 0 {
		t.Errorf("Count() = %d, want 0", q.Count())
	}
}

func TestQueue_Add(t *testing.T) {
	q := NewQueue("")
	
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://example.com/file.zip", false},
		{"valid https", "https://example.com/file.zip", false},
		{"empty", "", true},
		{"no scheme", "example.com/file.zip", true},
		{"comment", "# this is a comment", false}, // Comments are skipped, no error
		{"invalid url", "://invalid", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := q.Add(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestQueue_AddWithOptions(t *testing.T) {
	q := NewQueue("/downloads")
	
	err := q.AddWithOptions("https://example.com/file.zip", "custom.zip", "sha256:abc123")
	if err != nil {
		t.Fatalf("AddWithOptions() error = %v", err)
	}
	
	items := q.Items()
	if len(items) != 1 {
		t.Fatalf("Items() len = %d, want 1", len(items))
	}
	
	item := items[0]
	if item.URL != "https://example.com/file.zip" {
		t.Errorf("URL = %q, want https://example.com/file.zip", item.URL)
	}
	if item.OutputPath != "custom.zip" {
		t.Errorf("OutputPath = %q, want custom.zip", item.OutputPath)
	}
	if item.Checksum != "sha256:abc123" {
		t.Errorf("Checksum = %q, want sha256:abc123", item.Checksum)
	}
}

func TestQueue_LoadFromFile(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "urls.txt")
	
	content := `# Comment line
https://example.com/file1.zip
https://example.com/file2.zip output2.zip
https://example.com/file3.zip output3.zip sha256:abc123

# Pipe-separated format
https://example.com/file4.zip|output4.zip|md5:def456
`
	
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	
	q := NewQueue(tmpDir)
	if err := q.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	
	if q.Count() != 4 {
		t.Errorf("Count() = %d, want 4", q.Count())
	}
	
	items := q.Items()
	
	// Check first item (URL only)
	if items[0].OutputPath != filepath.Join(tmpDir, "file1.zip") {
		t.Errorf("Item[0].OutputPath = %q", items[0].OutputPath)
	}
	
	// Check second item (URL + output)
	if items[1].OutputPath != "output2.zip" {
		t.Errorf("Item[1].OutputPath = %q, want output2.zip", items[1].OutputPath)
	}
	
	// Check third item (URL + output + checksum)
	if items[2].Checksum != "sha256:abc123" {
		t.Errorf("Item[2].Checksum = %q, want sha256:abc123", items[2].Checksum)
	}
	
	// Check fourth item (pipe-separated)
	if items[3].OutputPath != "output4.zip" {
		t.Errorf("Item[3].OutputPath = %q, want output4.zip", items[3].OutputPath)
	}
	if items[3].Checksum != "md5:def456" {
		t.Errorf("Item[3].Checksum = %q, want md5:def456", items[3].Checksum)
	}
}

func TestQueue_LoadFromFile_NotFound(t *testing.T) {
	q := NewQueue("")
	err := q.LoadFromFile("/nonexistent/file.txt")
	if err == nil {
		t.Error("LoadFromFile() should return error for nonexistent file")
	}
}

func TestQueue_UpdateStatus(t *testing.T) {
	q := NewQueue("")
	q.Add("https://example.com/file.zip")
	
	q.UpdateStatus(0, QueueStatusDownloading)
	
	item := q.Get(0)
	if item.Status != QueueStatusDownloading {
		t.Errorf("Status = %v, want %v", item.Status, QueueStatusDownloading)
	}
	if item.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}
	
	q.UpdateStatus(0, QueueStatusCompleted)
	item = q.Get(0)
	if item.Status != QueueStatusCompleted {
		t.Errorf("Status = %v, want %v", item.Status, QueueStatusCompleted)
	}
	if item.EndTime.IsZero() {
		t.Error("EndTime should be set")
	}
}

func TestQueue_UpdateProgress(t *testing.T) {
	q := NewQueue("")
	q.Add("https://example.com/file.zip")
	
	q.UpdateProgress(0, 5000, 10000)
	
	item := q.Get(0)
	if item.Downloaded != 5000 {
		t.Errorf("Downloaded = %d, want 5000", item.Downloaded)
	}
	if item.TotalSize != 10000 {
		t.Errorf("TotalSize = %d, want 10000", item.TotalSize)
	}
}

func TestQueue_SetError(t *testing.T) {
	q := NewQueue("")
	q.Add("https://example.com/file.zip")
	
	testErr := os.ErrNotExist
	q.SetError(0, testErr)
	
	item := q.Get(0)
	if item.Status != QueueStatusFailed {
		t.Errorf("Status = %v, want %v", item.Status, QueueStatusFailed)
	}
	if item.Error != testErr {
		t.Errorf("Error = %v, want %v", item.Error, testErr)
	}
}

func TestQueue_NextPending(t *testing.T) {
	q := NewQueue("")
	q.Add("https://example.com/file1.zip")
	q.Add("https://example.com/file2.zip")
	
	// First pending
	item := q.NextPending()
	if item == nil {
		t.Fatal("NextPending() returned nil")
	}
	if item.ID != 0 {
		t.Errorf("ID = %d, want 0", item.ID)
	}
	
	// Mark first as completed
	q.UpdateStatus(0, QueueStatusCompleted)
	
	// Next pending should be second
	item = q.NextPending()
	if item.ID != 1 {
		t.Errorf("ID = %d, want 1", item.ID)
	}
	
	// Mark second as completed
	q.UpdateStatus(1, QueueStatusCompleted)
	
	// No more pending
	item = q.NextPending()
	if item != nil {
		t.Error("NextPending() should return nil when no pending items")
	}
}

func TestQueue_Stats(t *testing.T) {
	q := NewQueue("")
	q.Add("https://example.com/file1.zip")
	q.Add("https://example.com/file2.zip")
	q.Add("https://example.com/file3.zip")
	
	q.UpdateStatus(0, QueueStatusCompleted)
	q.UpdateProgress(0, 1000, 1000)
	
	q.UpdateStatus(1, QueueStatusDownloading)
	q.UpdateProgress(1, 500, 2000)
	
	stats := q.Stats()
	
	if stats.Total != 3 {
		t.Errorf("Total = %d, want 3", stats.Total)
	}
	if stats.Completed != 1 {
		t.Errorf("Completed = %d, want 1", stats.Completed)
	}
	if stats.Downloading != 1 {
		t.Errorf("Downloading = %d, want 1", stats.Downloading)
	}
	if stats.Pending != 1 {
		t.Errorf("Pending = %d, want 1", stats.Pending)
	}
	if stats.Downloaded != 1500 {
		t.Errorf("Downloaded = %d, want 1500", stats.Downloaded)
	}
}

func TestQueue_IsComplete(t *testing.T) {
	q := NewQueue("")
	q.Add("https://example.com/file.zip")
	
	if q.IsComplete() {
		t.Error("IsComplete() should be false with pending items")
	}
	
	q.UpdateStatus(0, QueueStatusCompleted)
	
	if !q.IsComplete() {
		t.Error("IsComplete() should be true when all completed")
	}
}

func TestQueue_Clear(t *testing.T) {
	q := NewQueue("")
	q.Add("https://example.com/file1.zip")
	q.Add("https://example.com/file2.zip")
	
	if q.Count() != 2 {
		t.Errorf("Count() = %d, want 2", q.Count())
	}
	
	q.Clear()
	
	if q.Count() != 0 {
		t.Errorf("Count() after Clear() = %d, want 0", q.Count())
	}
}

func TestQueueStatus_String(t *testing.T) {
	tests := []struct {
		status   QueueStatus
		expected string
	}{
		{QueueStatusPending, "pending"},
		{QueueStatusDownloading, "downloading"},
		{QueueStatusCompleted, "completed"},
		{QueueStatusFailed, "failed"},
		{QueueStatusSkipped, "skipped"},
		{QueueStatusCanceled, "canceled"},
		{QueueStatus(99), "unknown"},
	}
	
	for _, tt := range tests {
		if tt.status.String() != tt.expected {
			t.Errorf("%d.String() = %q, want %q", tt.status, tt.status.String(), tt.expected)
		}
	}
}

func TestExtractFilename(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/file.zip", "file.zip"},
		{"https://example.com/path/to/file.tar.gz", "file.tar.gz"},
		{"https://example.com/", "download"},
		{"https://example.com/file%20name.zip", "file name.zip"},
		{"://invalid", "download"},
	}
	
	for _, tt := range tests {
		got := extractFilename(tt.url)
		if got != tt.expected {
			t.Errorf("extractFilename(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}

func TestNewQueueManager(t *testing.T) {
	q := NewQueue("")
	qm := NewQueueManager(q, 4)
	
	if qm == nil {
		t.Fatal("NewQueueManager returned nil")
	}
	
	if qm.Queue() != q {
		t.Error("Queue() should return the same queue")
	}
	
	if qm.Context() == nil {
		t.Error("Context() should not be nil")
	}
	
	// Test Stop
	qm.Stop()
	
	select {
	case <-qm.Context().Done():
		// Expected
	default:
		t.Error("Context should be canceled after Stop()")
	}
}
