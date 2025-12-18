// Package protocol provides protocol adapters for different download protocols.
package protocol

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
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

// WithPinnedPublicKey sets certificate pinning using SHA256 hash of the public key
// The pin should be in format "sha256//base64hash" (like curl's --pinnedpubkey)
// Multiple pins can be provided separated by semicolons
func WithPinnedPublicKey(pins string) HTTPClientOption {
	return func(c *HTTPClient) {
		if pins == "" {
			return
		}

		// Parse pins
		pinList := parsePins(pins)
		if len(pinList) == 0 {
			return
		}

		transport := c.getTransport()
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}

		// Set custom verification function
		transport.TLSClientConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			// If no verified chains (InsecureSkipVerify), parse raw certs
			var certs []*x509.Certificate
			if len(verifiedChains) > 0 {
				for _, chain := range verifiedChains {
					certs = append(certs, chain...)
				}
			} else {
				for _, raw := range rawCerts {
					cert, err := x509.ParseCertificate(raw)
					if err != nil {
						continue
					}
					certs = append(certs, cert)
				}
			}

			// Check if any certificate matches any pin
			for _, cert := range certs {
				certPin := calculatePublicKeyPin(cert)
				for _, pin := range pinList {
					if certPin == pin {
						return nil // Match found
					}
				}
			}

			return fmt.Errorf("certificate public key does not match any pinned key")
		}
	}
}

// parsePins parses pin string in format "sha256//hash1;sha256//hash2"
func parsePins(pins string) []string {
	var result []string
	for _, pin := range strings.Split(pins, ";") {
		pin = strings.TrimSpace(pin)
		if pin == "" {
			continue
		}
		// Remove "sha256//" prefix if present
		if strings.HasPrefix(pin, "sha256//") {
			pin = pin[8:]
		} else if strings.HasPrefix(pin, "sha256/") {
			pin = pin[7:]
		}
		if pin != "" {
			result = append(result, pin)
		}
	}
	return result
}

// calculatePublicKeyPin calculates SHA256 hash of certificate's public key
// Returns base64-encoded hash
func calculatePublicKeyPin(cert *x509.Certificate) string {
	// Get the SubjectPublicKeyInfo (SPKI)
	spki := cert.RawSubjectPublicKeyInfo
	hash := sha256.Sum256(spki)
	return base64.StdEncoding.EncodeToString(hash[:])
}

// GetPublicKeyPin returns the SHA256 pin for a certificate (useful for debugging)
func GetPublicKeyPin(cert *x509.Certificate) string {
	return "sha256//" + calculatePublicKeyPin(cert)
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
// Supports RFC 2616 (Basic), RFC 5987 (UTF-8/encoded), and RFC 6266 standards
func parseContentDisposition(cd string) string {
	// Handle: attachment; filename="example.zip"
	// Handle: attachment; filename=example.zip
	// Handle: attachment; filename*=UTF-8''example.zip
	// Handle: attachment; filename*=utf-8'en'%E4%B8%AD%E6%96%87.zip
	// Handle: inline; filename="file.pdf"

	var filename string
	var filenameEncoded string

	parts := strings.Split(cd, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		lowerPart := strings.ToLower(part)

		// Check for filename*= (RFC 5987) - takes priority
		if strings.HasPrefix(lowerPart, "filename*=") {
			value := part[10:]
			// Handle encoding''filename format (e.g., UTF-8''name or UTF-8'en'name)
			if idx := strings.Index(value, "''"); idx >= 0 {
				value = value[idx+2:]
			} else if idx := strings.Index(value, "'"); idx >= 0 {
				// Handle single quote format (language tag)
				if idx2 := strings.Index(value[idx+1:], "'"); idx2 >= 0 {
					value = value[idx+1+idx2+1:]
				}
			}
			// URL decode
			if decoded, err := url.QueryUnescape(value); err == nil {
				filenameEncoded = decoded
			} else {
				filenameEncoded = value
			}
		}

		// Check for filename=
		if strings.HasPrefix(lowerPart, "filename=") && !strings.HasPrefix(lowerPart, "filename*=") {
			value := part[9:]
			// Remove surrounding quotes
			value = strings.Trim(value, `"'`)
			// Handle escaped quotes within filename
			value = strings.ReplaceAll(value, `\"`, `"`)
			value = strings.ReplaceAll(value, `\\`, `\`)
			filename = value
		}
	}

	// RFC 5987 filename* takes priority over filename
	if filenameEncoded != "" {
		return sanitizeFilename(filenameEncoded)
	}
	if filename != "" {
		return sanitizeFilename(filename)
	}

	return ""
}

// sanitizeFilename removes potentially dangerous characters from filename
func sanitizeFilename(name string) string {
	// Remove path separators to prevent directory traversal
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	
	// Remove null bytes
	name = strings.ReplaceAll(name, "\x00", "")
	
	// Remove leading/trailing whitespace and dots
	name = strings.TrimSpace(name)
	name = strings.Trim(name, ".")
	
	// Replace other problematic characters
	replacer := strings.NewReplacer(
		"<", "_",
		">", "_",
		":", "_",
		"\"", "_",
		"|", "_",
		"?", "_",
		"*", "_",
	)
	name = replacer.Replace(name)
	
	// Limit length
	if len(name) > 255 {
		ext := filepath.Ext(name)
		if len(ext) > 50 {
			ext = ext[:50]
		}
		name = name[:255-len(ext)] + ext
	}
	
	return name
}
