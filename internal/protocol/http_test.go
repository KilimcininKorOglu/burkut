package protocol

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHTTPClient_Supports(t *testing.T) {
	client := NewHTTPClient()

	tests := []struct {
		name     string
		rawURL   string
		expected bool
	}{
		{"http scheme", "http://example.com/file.zip", true},
		{"https scheme", "https://example.com/file.zip", true},
		{"HTTP uppercase", "HTTP://example.com/file.zip", true},
		{"HTTPS uppercase", "HTTPS://example.com/file.zip", true},
		{"ftp scheme", "ftp://example.com/file.zip", false},
		{"sftp scheme", "sftp://example.com/file.zip", false},
		{"no scheme", "example.com/file.zip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			got := client.Supports(u)
			if got != tt.expected {
				t.Errorf("Supports(%q) = %v, want %v", tt.rawURL, got, tt.expected)
			}
		})
	}
}

func TestHTTPClient_Head(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("Expected HEAD request, got %s", r.Method)
		}

		w.Header().Set("Content-Length", "1024")
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient()
	ctx := context.Background()

	meta, err := client.Head(ctx, server.URL+"/test.zip")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}

	if meta.ContentLength != 1024 {
		t.Errorf("ContentLength = %d, want 1024", meta.ContentLength)
	}

	if meta.ContentType != "application/zip" {
		t.Errorf("ContentType = %q, want %q", meta.ContentType, "application/zip")
	}

	if !meta.AcceptRanges {
		t.Error("AcceptRanges = false, want true")
	}

	if meta.ETag != `"abc123"` {
		t.Errorf("ETag = %q, want %q", meta.ETag, `"abc123"`)
	}

	if meta.Filename != "test.zip" {
		t.Errorf("Filename = %q, want %q", meta.Filename, "test.zip")
	}
}

func TestHTTPClient_Get(t *testing.T) {
	content := []byte("Hello, Burkut!")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		w.Header().Set("Content-Length", "14")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	client := NewHTTPClient()
	ctx := context.Background()

	body, meta, err := client.Get(ctx, server.URL+"/hello.txt")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if string(data) != string(content) {
		t.Errorf("Got %q, want %q", string(data), string(content))
	}

	if meta.Filename != "hello.txt" {
		t.Errorf("Filename = %q, want %q", meta.Filename, "hello.txt")
	}
}

func TestHTTPClient_GetRange(t *testing.T) {
	content := []byte("0123456789ABCDEF")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			t.Error("Expected Range header")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Parse range header: bytes=0-4
		var start, end int64
		_, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
		if err != nil {
			t.Errorf("Failed to parse range header: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(content[start : end+1])
	}))
	defer server.Close()

	client := NewHTTPClient()
	ctx := context.Background()

	// Request bytes 0-4 (should get "01234")
	body, err := client.GetRange(ctx, server.URL+"/file.bin", 0, 4)
	if err != nil {
		t.Fatalf("GetRange() error = %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	expected := "01234"
	if string(data) != expected {
		t.Errorf("Got %q, want %q", string(data), expected)
	}
}

func TestHTTPClient_GetRange_NoSupport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server ignores Range header and returns full content with 200 OK
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("full content"))
	}))
	defer server.Close()

	client := NewHTTPClient()
	ctx := context.Background()

	_, err := client.GetRange(ctx, server.URL+"/file.bin", 0, 100)
	if err == nil {
		t.Error("Expected error when server doesn't support ranges")
	}
}

func TestHTTPClient_WithOptions(t *testing.T) {
	client := NewHTTPClient(
		WithTimeout(60*time.Second),
		WithUserAgent("TestAgent/1.0"),
		WithHeader("X-Custom", "value"),
	)

	if client.userAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %q, want %q", client.userAgent, "TestAgent/1.0")
	}

	if client.headers["X-Custom"] != "value" {
		t.Errorf("Custom header not set correctly")
	}
}

func TestHTTPClient_ProtocolVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient()
	ctx := context.Background()

	meta, err := client.Head(ctx, server.URL+"/test.bin")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}

	// httptest.NewServer uses HTTP/1.1
	if meta.Protocol != "HTTP/1.1" {
		t.Errorf("Protocol = %q, want %q", meta.Protocol, "HTTP/1.1")
	}
}

func TestHTTPClient_ForceHTTP1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(WithForceHTTP1(true))
	ctx := context.Background()

	meta, err := client.Head(ctx, server.URL+"/test.bin")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}

	// With ForceHTTP1, should always use HTTP/1.1
	if meta.Protocol != "HTTP/1.1" {
		t.Errorf("Protocol = %q, want %q", meta.Protocol, "HTTP/1.1")
	}

	// Verify that forceHTTP1 flag is set
	if !client.forceHTTP1 {
		t.Error("forceHTTP1 flag should be true")
	}
}

func TestHTTPClient_ForceHTTP2(t *testing.T) {
	// Test that ForceHTTP2 option sets the flag
	client := NewHTTPClient(WithForceHTTP2(true))

	if !client.forceHTTP2 {
		t.Error("forceHTTP2 flag should be true")
	}

	// Verify transport has ForceAttemptHTTP2 set
	transport := client.getTransport()
	if !transport.ForceAttemptHTTP2 {
		t.Error("transport.ForceAttemptHTTP2 should be true")
	}
}

func TestHTTPClient_ForceHTTP1_DisablesHTTP2(t *testing.T) {
	client := NewHTTPClient(WithForceHTTP1(true))

	transport := client.getTransport()

	// Verify TLSNextProto is empty (disables HTTP/2)
	if transport.TLSNextProto == nil {
		t.Error("TLSNextProto should be initialized to empty map")
	}

	if len(transport.TLSNextProto) != 0 {
		t.Error("TLSNextProto should be empty to disable HTTP/2")
	}

	// Verify ForceAttemptHTTP2 is false
	if transport.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be false when ForceHTTP1 is enabled")
	}
}

func TestParsePins(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "sha256//abc123",
			expected: []string{"abc123"},
		},
		{
			input:    "sha256/abc123",
			expected: []string{"abc123"},
		},
		{
			input:    "abc123",
			expected: []string{"abc123"},
		},
		{
			input:    "sha256//abc123;sha256//def456",
			expected: []string{"abc123", "def456"},
		},
		{
			input:    "sha256//abc123; sha256//def456 ; sha256//ghi789",
			expected: []string{"abc123", "def456", "ghi789"},
		},
		{
			input:    "",
			expected: nil,
		},
		{
			input:    ";;;",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parsePins(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parsePins(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("parsePins(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestWithPinnedPublicKey_Empty(t *testing.T) {
	// Should not set any verification with empty pin
	client := NewHTTPClient(WithPinnedPublicKey(""))

	transport := client.getTransport()
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.VerifyPeerCertificate != nil {
		t.Error("VerifyPeerCertificate should be nil with empty pin")
	}
}

func TestWithPinnedPublicKey_SetsVerification(t *testing.T) {
	client := NewHTTPClient(WithPinnedPublicKey("sha256//abc123"))

	transport := client.getTransport()
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig should not be nil")
	}
	if transport.TLSClientConfig.VerifyPeerCertificate == nil {
		t.Error("VerifyPeerCertificate should be set")
	}
}

func TestExtractFilename(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		disposition string
		expected    string
	}{
		{
			name:     "simple URL",
			url:      "http://example.com/file.zip",
			expected: "file.zip",
		},
		{
			name:     "URL with path",
			url:      "http://example.com/path/to/document.pdf",
			expected: "document.pdf",
		},
		{
			name:     "URL with query string",
			url:      "http://example.com/file.zip?token=abc",
			expected: "file.zip",
		},
		{
			name:     "URL encoded filename",
			url:      "http://example.com/my%20file.zip",
			expected: "my file.zip",
		},
		{
			name:        "Content-Disposition with quotes",
			url:         "http://example.com/download",
			disposition: `attachment; filename="report.pdf"`,
			expected:    "report.pdf",
		},
		{
			name:        "Content-Disposition without quotes",
			url:         "http://example.com/download",
			disposition: "attachment; filename=report.pdf",
			expected:    "report.pdf",
		},
		{
			name:        "Content-Disposition UTF-8",
			url:         "http://example.com/download",
			disposition: "attachment; filename*=UTF-8''%C3%B6rnek.pdf",
			expected:    "örnek.pdf",
		},
		{
			name:     "empty path",
			url:      "http://example.com/",
			expected: "download",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.disposition != "" {
					w.Header().Set("Content-Disposition", tt.disposition)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			// Construct the test URL
			testURL := server.URL
			if u, err := url.Parse(tt.url); err == nil {
				testURL = server.URL + u.Path
				if u.RawQuery != "" {
					testURL += "?" + u.RawQuery
				}
			}

			client := NewHTTPClient()
			meta, err := client.Head(context.Background(), testURL)
			if err != nil {
				t.Fatalf("Head() error = %v", err)
			}

			if meta.Filename != tt.expected {
				t.Errorf("Filename = %q, want %q", meta.Filename, tt.expected)
			}
		})
	}
}

func TestParseContentDisposition(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic attachment",
			input:    `attachment; filename="file.zip"`,
			expected: "file.zip",
		},
		{
			name:     "without quotes",
			input:    "attachment; filename=file.zip",
			expected: "file.zip",
		},
		{
			name:     "inline disposition",
			input:    `inline; filename="document.pdf"`,
			expected: "document.pdf",
		},
		{
			name:     "UTF-8 encoded",
			input:    "attachment; filename*=UTF-8''%E4%B8%AD%E6%96%87.txt",
			expected: "中文.txt",
		},
		{
			name:     "UTF-8 with language tag",
			input:    "attachment; filename*=UTF-8'en'%E4%B8%AD%E6%96%87.txt",
			expected: "中文.txt",
		},
		{
			name:     "both filename and filename*",
			input:    `attachment; filename="fallback.txt"; filename*=UTF-8''%E4%B8%AD%E6%96%87.txt`,
			expected: "中文.txt", // filename* takes priority
		},
		{
			name:     "escaped quotes",
			input:    `attachment; filename="file\"with\"quotes.txt"`,
			expected: "file_with_quotes.txt", // quotes are sanitized
		},
		{
			name:     "path traversal attempt",
			input:    `attachment; filename="../../../etc/passwd"`,
			expected: "_.._.._etc_passwd", // dots between path separators preserved
		},
		{
			name:     "null byte attack",
			input:    "attachment; filename=\"file.txt\x00.exe\"",
			expected: "file.txt.exe",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "no filename",
			input:    "attachment",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseContentDisposition(tt.input)
			if result != tt.expected {
				t.Errorf("parseContentDisposition(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal", "file.txt", "file.txt"},
		{"with spaces", "my file.txt", "my file.txt"},
		{"path separator slash", "path/to/file.txt", "path_to_file.txt"},
		{"path separator backslash", "path\\to\\file.txt", "path_to_file.txt"},
		{"leading dots", "...file.txt", "file.txt"},
		{"trailing dots", "file.txt...", "file.txt"},
		{"special chars", "file<>:\"|?*.txt", "file_______.txt"},
		{"very long name", strings.Repeat("a", 300) + ".txt", strings.Repeat("a", 251) + ".txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}


