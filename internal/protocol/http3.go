// Package protocol provides protocol adapters for different download protocols.
package protocol

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// HTTP3Client is an HTTP/3 (QUIC) protocol adapter
type HTTP3Client struct {
	client    *http.Client
	userAgent string
	headers   map[string]string
}

// HTTP3ClientOption configures HTTP3Client
type HTTP3ClientOption func(*HTTP3Client)

// WithHTTP3Timeout sets the timeout
func WithHTTP3Timeout(timeout time.Duration) HTTP3ClientOption {
	return func(c *HTTP3Client) {
		c.client.Timeout = timeout
	}
}

// WithHTTP3UserAgent sets the User-Agent
func WithHTTP3UserAgent(ua string) HTTP3ClientOption {
	return func(c *HTTP3Client) {
		c.userAgent = ua
	}
}

// WithHTTP3Header adds a custom header
func WithHTTP3Header(key, value string) HTTP3ClientOption {
	return func(c *HTTP3Client) {
		c.headers[key] = value
	}
}

// NewHTTP3Client creates a new HTTP/3 client
func NewHTTP3Client(opts ...HTTP3ClientOption) *HTTP3Client {
	// Create HTTP/3 round tripper
	roundTripper := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	c := &HTTP3Client{
		client: &http.Client{
			Transport: roundTripper,
			Timeout:   30 * time.Second,
		},
		userAgent: "Burkut/0.1 (HTTP/3)",
		headers:   make(map[string]string),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Supports checks if this client supports the URL
func (c *HTTP3Client) Supports(u *url.URL) bool {
	// HTTP/3 only works with HTTPS
	return u.Scheme == "https"
}

// Head fetches metadata without downloading
func (c *HTTP3Client) Head(ctx context.Context, rawURL string) (*Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating HEAD request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing HEAD request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HEAD request failed: %s", resp.Status)
	}

	return parseHTTP3Metadata(rawURL, resp)
}

// Get downloads the entire file
func (c *HTTP3Client) Get(ctx context.Context, rawURL string) (io.ReadCloser, *Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating GET request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("executing GET request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("GET request failed: %s", resp.Status)
	}

	meta, err := parseHTTP3Metadata(rawURL, resp)
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}

	return resp.Body, meta, nil
}

// GetRange downloads a byte range
func (c *HTTP3Client) GetRange(ctx context.Context, rawURL string, start, end int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating range GET request: %w", err)
	}

	c.setHeaders(req)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing range GET request: %w", err)
	}

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("range GET request failed: %s", resp.Status)
	}

	if resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server does not support range requests")
	}

	return resp.Body, nil
}

// setHeaders sets common headers
func (c *HTTP3Client) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")

	for key, value := range c.headers {
		req.Header.Set(key, value)
	}
}

// parseHTTP3Metadata extracts metadata from response
func parseHTTP3Metadata(rawURL string, resp *http.Response) (*Metadata, error) {
	meta := &Metadata{
		URL:         rawURL,
		ContentType: resp.Header.Get("Content-Type"),
		ETag:        resp.Header.Get("ETag"),
		AcceptRanges: resp.Header.Get("Accept-Ranges") == "bytes",
	}

	// Content-Length
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		var length int64
		fmt.Sscanf(cl, "%d", &length)
		meta.ContentLength = length
	}

	// Last-Modified
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			meta.LastModified = t
		}
	}

	// Filename
	meta.Filename = extractFilenameFromURL(rawURL, resp)

	return meta, nil
}

// extractFilenameFromURL extracts filename from URL or Content-Disposition
func extractFilenameFromURL(rawURL string, resp *http.Response) string {
	// Try Content-Disposition
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if filename := parseContentDisposition(cd); filename != "" {
			return filename
		}
	}

	// Fall back to URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return "download"
	}

	path := u.Path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			path = path[i+1:]
			break
		}
	}

	if path == "" {
		return "download"
	}

	return path
}

// Close closes the HTTP/3 client
func (c *HTTP3Client) Close() error {
	if t, ok := c.client.Transport.(*http3.Transport); ok {
		return t.Close()
	}
	return nil
}

// IsHTTP3Available checks if HTTP/3 is supported by probing the server
func IsHTTP3Available(ctx context.Context, rawURL string) bool {
	client := NewHTTP3Client()
	defer client.Close()

	// Try a HEAD request
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := client.Head(ctx, rawURL)
	return err == nil
}
