// Package protocol provides protocol adapters for different download protocols.
package protocol

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// Metadata contains information about a remote file
type Metadata struct {
	URL           string
	Filename      string
	ContentLength int64
	AcceptRanges  bool
	ContentType   string
	LastModified  time.Time
	ETag          string
	Protocol      string // HTTP protocol version (e.g., "HTTP/1.1", "HTTP/2.0")
}

// HTTPClient is an HTTP protocol adapter for downloading files
type HTTPClient struct {
	client     *http.Client
	userAgent  string
	headers    map[string]string
	forceHTTP1 bool // Force HTTP/1.1 instead of HTTP/2
	forceHTTP2 bool // Force HTTP/2 (fail if not supported)
}

// HTTPClientOption is a function that configures HTTPClient
type HTTPClientOption func(*HTTPClient)

// WithTimeout sets the HTTP client timeout
func WithTimeout(timeout time.Duration) HTTPClientOption {
	return func(c *HTTPClient) {
		c.client.Timeout = timeout
	}
}

// WithUserAgent sets the User-Agent header
func WithUserAgent(ua string) HTTPClientOption {
	return func(c *HTTPClient) {
		c.userAgent = ua
	}
}

// WithHeader adds a custom header
func WithHeader(key, value string) HTTPClientOption {
	return func(c *HTTPClient) {
		c.headers[key] = value
	}
}

// WithHeaders adds multiple custom headers
func WithHeaders(headers map[string]string) HTTPClientOption {
	return func(c *HTTPClient) {
		for key, value := range headers {
			c.headers[key] = value
		}
	}
}

// WithBasicAuth sets Basic authentication
func WithBasicAuth(username, password string) HTTPClientOption {
	return func(c *HTTPClient) {
		if username != "" {
			c.headers["Authorization"] = "Basic " + basicAuth(username, password)
		}
	}
}

// basicAuth encodes username and password for Basic auth header
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64Encode([]byte(auth))
}

// base64Encode encodes bytes to base64 string
func base64Encode(data []byte) string {
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := make([]byte, 0, (len(data)+2)/3*4)

	for i := 0; i < len(data); i += 3 {
		var n uint32
		remaining := len(data) - i

		if remaining >= 3 {
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			result = append(result, base64Chars[n>>18], base64Chars[(n>>12)&0x3F], base64Chars[(n>>6)&0x3F], base64Chars[n&0x3F])
		} else if remaining == 2 {
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8
			result = append(result, base64Chars[n>>18], base64Chars[(n>>12)&0x3F], base64Chars[(n>>6)&0x3F], '=')
		} else {
			n = uint32(data[i]) << 16
			result = append(result, base64Chars[n>>18], base64Chars[(n>>12)&0x3F], '=', '=')
		}
	}

	return string(result)
}

// WithTransport sets a custom transport (useful for proxy)
func WithTransport(transport *http.Transport) HTTPClientOption {
	return func(c *HTTPClient) {
		c.client.Transport = transport
	}
}

// WithProxy sets an HTTP or HTTPS proxy
func WithProxy(proxyURL string) HTTPClientOption {
	return func(c *HTTPClient) {
		if proxyURL == "" {
			return
		}

		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return
		}

		transport := c.getTransport()
		transport.Proxy = http.ProxyURL(parsed)
	}
}

// WithSOCKS5Proxy sets a SOCKS5 proxy
func WithSOCKS5Proxy(proxyAddr string, auth *proxy.Auth) HTTPClientOption {
	return func(c *HTTPClient) {
		if proxyAddr == "" {
			return
		}

		// Parse socks5:// URL if provided
		if strings.HasPrefix(proxyAddr, "socks5://") {
			parsed, err := url.Parse(proxyAddr)
			if err != nil {
				return
			}
			proxyAddr = parsed.Host
			if parsed.User != nil {
				password, _ := parsed.User.Password()
				auth = &proxy.Auth{
					User:     parsed.User.Username(),
					Password: password,
				}
			}
		}

		dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
		if err != nil {
			return
		}

		transport := c.getTransport()
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	}
}

// WithInsecureSkipVerify disables TLS certificate verification
func WithInsecureSkipVerify(skip bool) HTTPClientOption {
	return func(c *HTTPClient) {
		if !skip {
			return
		}
		transport := c.getTransport()
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}
}

// WithTLSConfig sets custom TLS configuration
func WithTLSConfig(config *tls.Config) HTTPClientOption {
	return func(c *HTTPClient) {
		if config == nil {
			return
		}
		transport := c.getTransport()
		transport.TLSClientConfig = config
	}
}

// WithForceHTTP1 forces HTTP/1.1 instead of HTTP/2
// Useful for servers that have issues with HTTP/2
func WithForceHTTP1(force bool) HTTPClientOption {
	return func(c *HTTPClient) {
		c.forceHTTP1 = force
		if force {
			transport := c.getTransport()
			// Disable HTTP/2 by setting TLSNextProto to empty map
			transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
			// Also disable ForceAttemptHTTP2
			transport.ForceAttemptHTTP2 = false
		}
	}
}

// WithForceHTTP2 forces HTTP/2 and fails if server doesn't support it
func WithForceHTTP2(force bool) HTTPClientOption {
	return func(c *HTTPClient) {
		c.forceHTTP2 = force
		if force {
			transport := c.getTransport()
			transport.ForceAttemptHTTP2 = true
		}
	}
}

// getTransport returns the underlying transport, creating one if needed
func (c *HTTPClient) getTransport() *http.Transport {
	if t, ok := c.client.Transport.(*http.Transport); ok {
		return t
	}
	// Create new transport and set it
	t := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	c.client.Transport = t
	return t
}

// NewHTTPClient creates a new HTTP client with the given options
func NewHTTPClient(opts ...HTTPClientOption) *HTTPClient {
	c := &HTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		userAgent: "Burkut/0.1",
		headers:   make(map[string]string),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Supports checks if the URL is supported by this protocol
func (c *HTTPClient) Supports(u *url.URL) bool {
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}

// Head fetches metadata about the file without downloading it
func (c *HTTPClient) Head(ctx context.Context, rawURL string) (*Metadata, error) {
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

	return c.parseMetadata(rawURL, resp)
}

// Get downloads the entire file
func (c *HTTPClient) Get(ctx context.Context, rawURL string) (io.ReadCloser, *Metadata, error) {
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

	meta, err := c.parseMetadata(rawURL, resp)
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}

	return resp.Body, meta, nil
}

// GetRange downloads a specific byte range of the file
func (c *HTTPClient) GetRange(ctx context.Context, rawURL string, start, end int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating range GET request: %w", err)
	}

	c.setHeaders(req)

	// Set Range header for partial content
	// Range is inclusive on both ends: bytes=0-99 fetches first 100 bytes
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing range GET request: %w", err)
	}

	// 206 Partial Content is expected for range requests
	// 200 OK means server doesn't support ranges (will send full file)
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("range GET request failed: %s", resp.Status)
	}

	// If server returned 200 instead of 206, it doesn't support ranges
	if resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server does not support range requests")
	}

	return resp.Body, nil
}

// setHeaders sets common headers on the request
func (c *HTTPClient) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity") // Don't accept compressed responses for downloads

	for key, value := range c.headers {
		req.Header.Set(key, value)
	}
}

// parseMetadata extracts file metadata from HTTP response
func (c *HTTPClient) parseMetadata(rawURL string, resp *http.Response) (*Metadata, error) {
	meta := &Metadata{
		URL:          rawURL,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		AcceptRanges: strings.ToLower(resp.Header.Get("Accept-Ranges")) == "bytes",
		Protocol:     resp.Proto, // e.g., "HTTP/1.1", "HTTP/2.0"
	}

	// Parse Content-Length
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		length, err := strconv.ParseInt(cl, 10, 64)
		if err == nil {
			meta.ContentLength = length
		}
	}

	// Parse Last-Modified
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			meta.LastModified = t
		}
	}

	// Extract filename from Content-Disposition or URL
	meta.Filename = c.extractFilename(rawURL, resp)

	// Check if HTTP/2 was forced but not used
	if c.forceHTTP2 && !strings.HasPrefix(resp.Proto, "HTTP/2") {
		return nil, fmt.Errorf("HTTP/2 was forced but server responded with %s", resp.Proto)
	}

	return meta, nil
}

// extractFilename extracts filename from Content-Disposition header or URL
func (c *HTTPClient) extractFilename(rawURL string, resp *http.Response) string {
	// Try Content-Disposition header first
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if filename := parseContentDisposition(cd); filename != "" {
			return filename
		}
	}

	// Fall back to URL path
	u, err := url.Parse(rawURL)
	if err != nil {
		return "download"
	}

	// Get the last segment of the path
	path := u.Path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[idx+1:]
	}

	// Remove query string if present
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	// URL decode the filename
	if decoded, err := url.QueryUnescape(path); err == nil {
		path = decoded
	}

	if path == "" {
		return "download"
	}

	return path
}

// parseContentDisposition extracts filename from Content-Disposition header
func parseContentDisposition(cd string) string {
	// Handle: attachment; filename="example.zip"
	// Handle: attachment; filename=example.zip
	// Handle: attachment; filename*=UTF-8''example.zip

	parts := strings.Split(cd, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check for filename*= (RFC 5987)
		if strings.HasPrefix(strings.ToLower(part), "filename*=") {
			value := part[10:]
			// Handle UTF-8''filename format
			if idx := strings.Index(value, "''"); idx >= 0 {
				value = value[idx+2:]
			}
			if decoded, err := url.QueryUnescape(value); err == nil {
				return decoded
			}
			return value
		}

		// Check for filename=
		if strings.HasPrefix(strings.ToLower(part), "filename=") {
			value := part[9:]
			// Remove surrounding quotes
			value = strings.Trim(value, `"'`)
			return value
		}
	}

	return ""
}
