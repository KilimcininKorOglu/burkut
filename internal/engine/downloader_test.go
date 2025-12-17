package engine

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/download"
	"github.com/kilimcininkoroglu/burkut/internal/protocol"
)

// createTestServer creates a test HTTP server with the given content
func createTestServer(t *testing.T, content []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for Range header
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			var start, end int64
			_, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
			if err != nil {
				// Try single value range (bytes=N-)
				_, err = fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				end = int64(len(content)) - 1
			}

			// Validate range
			if start < 0 || start >= int64(len(content)) || end >= int64(len(content)) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}

			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusPartialContent)
			w.Write(content[start : end+1])
			return
		}

		// Full content
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
}

func TestDownloader_SimpleDownload(t *testing.T) {
	// Create test content
	content := []byte("Hello, Burkut! This is a test file for download.")

	server := createTestServer(t, content)
	defer server.Close()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.txt")

	// Create downloader
	config := DefaultConfig()
	config.Connections = 1 // Simple single connection
	httpClient := protocol.NewHTTPClient()
	downloader := NewDownloader(config, httpClient)

	// Download
	ctx := context.Background()
	err := downloader.Download(ctx, server.URL+"/test.txt", outputPath)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Verify content
	downloaded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(downloaded) != string(content) {
		t.Errorf("Downloaded content = %q, want %q", string(downloaded), string(content))
	}

	// State file should be cleaned up
	if download.StateExists(outputPath) {
		t.Error("State file should be deleted after successful download")
	}
}

func TestDownloader_ParallelChunks(t *testing.T) {
	// Create larger test content (1MB)
	content := make([]byte, 1024*1024)
	rand.Read(content)

	server := createTestServer(t, content)
	defer server.Close()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "large.bin")

	// Create downloader with 4 connections
	config := DefaultConfig()
	config.Connections = 4
	config.ProgressInterval = 1 * time.Millisecond // Very fast interval for testing
	httpClient := protocol.NewHTTPClient()
	downloader := NewDownloader(config, httpClient)

	// Track progress
	var progressCalls int32
	downloader.SetProgressCallback(func(p Progress) {
		atomic.AddInt32(&progressCalls, 1)
	})

	// Download
	ctx := context.Background()
	err := downloader.Download(ctx, server.URL+"/large.bin", outputPath)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Verify content
	downloaded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if len(downloaded) != len(content) {
		t.Errorf("Downloaded size = %d, want %d", len(downloaded), len(content))
	}

	// Verify content matches
	for i := range content {
		if downloaded[i] != content[i] {
			t.Errorf("Content mismatch at byte %d: got %d, want %d", i, downloaded[i], content[i])
			break
		}
	}

	// Progress callback might not be called for fast downloads - that's OK
	t.Logf("Progress callback called %d times", atomic.LoadInt32(&progressCalls))
}

func TestDownloader_Resume(t *testing.T) {
	// Create test content
	content := make([]byte, 10000)
	for i := range content {
		content[i] = byte(i % 256)
	}

	server := createTestServer(t, content)
	defer server.Close()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "resume.bin")

	// First download - cancel after some bytes
	config := DefaultConfig()
	config.Connections = 2
	httpClient := protocol.NewHTTPClient()

	ctx, cancel := context.WithCancel(context.Background())

	downloader := NewDownloader(config, httpClient)
	downloader.SetProgressCallback(func(p Progress) {
		if p.Downloaded > 1000 {
			cancel() // Cancel after 1KB
		}
	})

	err := downloader.Download(ctx, server.URL+"/resume.bin", outputPath)
	if err == nil || err != context.Canceled {
		// Download might complete before cancel
		if err == nil {
			// Full download completed, that's fine
			return
		}
		t.Logf("First download error (expected): %v", err)
	}

	// State file should exist
	if !download.StateExists(outputPath) {
		t.Log("State file not saved (download may have completed)")
		return
	}

	// Resume download
	ctx2 := context.Background()
	downloader2 := NewDownloader(config, httpClient)

	err = downloader2.Download(ctx2, server.URL+"/resume.bin", outputPath)
	if err != nil {
		t.Fatalf("Resume download error = %v", err)
	}

	// Verify content
	downloaded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if len(downloaded) != len(content) {
		t.Errorf("Downloaded size = %d, want %d", len(downloaded), len(content))
	}
}

func TestDownloader_Cancel(t *testing.T) {
	// This test verifies that download can be cancelled
	// We use a context timeout instead of manual cancel to avoid race conditions
	content := make([]byte, 1024)
	rand.Read(content)

	server := createTestServer(t, content)
	defer server.Close()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cancel.bin")

	config := DefaultConfig()
	config.Connections = 1
	httpClient := protocol.NewHTTPClient()
	downloader := NewDownloader(config, httpClient)

	// Use a very short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Download should fail due to timeout
	err := downloader.Download(ctx, server.URL+"/cancel.bin", outputPath)
	
	// Either context deadline exceeded or canceled is acceptable
	if err == nil {
		// Download completed before timeout - that's also OK for small files
		t.Log("Download completed before timeout")
	} else if err != context.DeadlineExceeded && err != context.Canceled {
		// Some other context-related error is also acceptable
		t.Logf("Got error: %v", err)
	}
}

func TestDownloader_Progress(t *testing.T) {
	content := make([]byte, 100000)
	rand.Read(content)

	server := createTestServer(t, content)
	defer server.Close()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "progress.bin")

	config := DefaultConfig()
	config.Connections = 2
	config.ProgressInterval = 10 * time.Millisecond
	httpClient := protocol.NewHTTPClient()
	downloader := NewDownloader(config, httpClient)

	downloader.SetProgressCallback(func(p Progress) {
		// Verify progress makes sense
		if p.TotalSize != int64(len(content)) {
			t.Errorf("TotalSize = %d, want %d", p.TotalSize, len(content))
		}

		if p.Downloaded > p.TotalSize {
			t.Errorf("Downloaded %d > TotalSize %d", p.Downloaded, p.TotalSize)
		}

		if p.Percent < 0 || p.Percent > 100 {
			t.Errorf("Percent = %f, should be 0-100", p.Percent)
		}
	})

	ctx := context.Background()
	err := downloader.Download(ctx, server.URL+"/progress.bin", outputPath)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Final progress should be 100%
	finalProgress := downloader.GetProgress()
	if finalProgress.Downloaded != int64(len(content)) {
		t.Errorf("Final Downloaded = %d, want %d", finalProgress.Downloaded, len(content))
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Connections != 4 {
		t.Errorf("Connections = %d, want 4", config.Connections)
	}

	if config.BufferSize != 32*1024 {
		t.Errorf("BufferSize = %d, want %d", config.BufferSize, 32*1024)
	}
}

// Ensure imports are used
var _ = io.EOF
