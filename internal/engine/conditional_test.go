package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/protocol"
)

func TestCheckTimestamp_FileNotExists(t *testing.T) {
	meta := &protocol.Metadata{
		LastModified:  time.Now(),
		ContentLength: 1000,
	}

	result, err := CheckTimestamp("/nonexistent/path/file.txt", meta)
	if err != nil {
		t.Fatalf("CheckTimestamp() error = %v", err)
	}

	if !result.ShouldDownload {
		t.Error("ShouldDownload should be true for non-existent file")
	}

	if result.Reason != "local file does not exist" {
		t.Errorf("Reason = %q, want %q", result.Reason, "local file does not exist")
	}
}

func TestCheckTimestamp_LocalNewer(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "test.txt")

	content := []byte("test content")
	if err := os.WriteFile(localPath, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set remote time to be older than local
	remoteTime := time.Now().Add(-24 * time.Hour)

	meta := &protocol.Metadata{
		LastModified:  remoteTime,
		ContentLength: int64(len(content)),
	}

	result, err := CheckTimestamp(localPath, meta)
	if err != nil {
		t.Fatalf("CheckTimestamp() error = %v", err)
	}

	if result.ShouldDownload {
		t.Error("ShouldDownload should be false when local file is newer")
	}
}

func TestCheckTimestamp_RemoteNewer(t *testing.T) {
	// Create a temporary file with old modification time
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "test.txt")

	content := []byte("test content")
	if err := os.WriteFile(localPath, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set local file to be old
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(localPath, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set file time: %v", err)
	}

	// Remote is newer
	remoteTime := time.Now()

	meta := &protocol.Metadata{
		LastModified:  remoteTime,
		ContentLength: int64(len(content)),
	}

	result, err := CheckTimestamp(localPath, meta)
	if err != nil {
		t.Fatalf("CheckTimestamp() error = %v", err)
	}

	if !result.ShouldDownload {
		t.Error("ShouldDownload should be true when remote file is newer")
	}

	if result.Reason != "remote file is newer than local file" {
		t.Errorf("Reason = %q, want %q", result.Reason, "remote file is newer than local file")
	}
}

func TestCheckTimestamp_NoLastModified(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(localPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Remote has no Last-Modified
	meta := &protocol.Metadata{
		ContentLength: 1000,
	}

	result, err := CheckTimestamp(localPath, meta)
	if err != nil {
		t.Fatalf("CheckTimestamp() error = %v", err)
	}

	if !result.ShouldDownload {
		t.Error("ShouldDownload should be true when server has no Last-Modified")
	}

	if result.Reason != "server did not provide Last-Modified header" {
		t.Errorf("Reason = %q, want %q", result.Reason, "server did not provide Last-Modified header")
	}
}

func TestCheckTimestamp_SizeMismatch(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "test.txt")

	content := []byte("short")
	if err := os.WriteFile(localPath, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Remote is older but different size
	remoteTime := time.Now().Add(-24 * time.Hour)

	meta := &protocol.Metadata{
		LastModified:  remoteTime,
		ContentLength: 1000, // Different from local
	}

	result, err := CheckTimestamp(localPath, meta)
	if err != nil {
		t.Fatalf("CheckTimestamp() error = %v", err)
	}

	if !result.ShouldDownload {
		t.Error("ShouldDownload should be true when size differs")
	}

	if result.Reason != "local file has different size than remote" {
		t.Errorf("Reason = %q, want %q", result.Reason, "local file has different size than remote")
	}
}

func TestCheckETag_Match(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(localPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	meta := &protocol.Metadata{
		ETag: `"abc123"`,
	}

	result, err := CheckETag(localPath, `"abc123"`, meta)
	if err != nil {
		t.Fatalf("CheckETag() error = %v", err)
	}

	if result.ShouldDownload {
		t.Error("ShouldDownload should be false when ETags match")
	}
}

func TestCheckETag_Mismatch(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(localPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	meta := &protocol.Metadata{
		ETag: `"xyz789"`,
	}

	result, err := CheckETag(localPath, `"abc123"`, meta)
	if err != nil {
		t.Fatalf("CheckETag() error = %v", err)
	}

	if !result.ShouldDownload {
		t.Error("ShouldDownload should be true when ETags differ")
	}

	if result.Reason != "ETag mismatch, remote file has changed" {
		t.Errorf("Reason = %q, want %q", result.Reason, "ETag mismatch, remote file has changed")
	}
}

func TestCheckETag_NoRemoteETag(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(localPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	meta := &protocol.Metadata{
		// No ETag
	}

	result, err := CheckETag(localPath, `"abc123"`, meta)
	if err != nil {
		t.Fatalf("CheckETag() error = %v", err)
	}

	if !result.ShouldDownload {
		t.Error("ShouldDownload should be true when server has no ETag")
	}
}

func TestNormalizeETag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"abc123"`, "abc123"},
		{`'abc123'`, "abc123"},
		{`abc123`, "abc123"},
		{`W/"abc123"`, "abc123"},
		{`W/"weak-etag"`, "weak-etag"},
		{`""`, ""},
		{``, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeETag(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeETag(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSetFileModTime(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(localPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set a specific time
	targetTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	if err := SetFileModTime(localPath, targetTime); err != nil {
		t.Fatalf("SetFileModTime() error = %v", err)
	}

	// Verify
	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Check modification time (with some tolerance)
	diff := info.ModTime().Sub(targetTime)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("File mod time = %v, want %v", info.ModTime(), targetTime)
	}
}

func TestSetFileModTime_ZeroTime(t *testing.T) {
	// Should not error with zero time
	err := SetFileModTime("/any/path", time.Time{})
	if err != nil {
		t.Errorf("SetFileModTime() with zero time should not error, got %v", err)
	}
}

func TestETagStore(t *testing.T) {
	store := NewETagStore("")

	// Test Set and Get
	store.Set("http://example.com/file.zip", `"abc123"`)

	etag := store.Get("http://example.com/file.zip")
	if etag != `"abc123"` {
		t.Errorf("Get() = %q, want %q", etag, `"abc123"`)
	}

	// Test Get non-existent
	etag = store.Get("http://example.com/other.zip")
	if etag != "" {
		t.Errorf("Get() for non-existent = %q, want empty", etag)
	}

	// Test Delete
	store.Delete("http://example.com/file.zip")
	etag = store.Get("http://example.com/file.zip")
	if etag != "" {
		t.Errorf("Get() after Delete = %q, want empty", etag)
	}
}
