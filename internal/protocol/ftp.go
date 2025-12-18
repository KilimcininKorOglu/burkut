// Package protocol provides protocol adapters for different download protocols.
package protocol

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

// FTPClient is an FTP protocol adapter for downloading files
type FTPClient struct {
	timeout        time.Duration
	username       string
	password       string
	passive        bool
	useTLS         bool          // Enable explicit FTPS (AUTH TLS)
	tlsConfig      *tls.Config   // Custom TLS configuration
	implicitTLS    bool          // Use implicit TLS (port 990)
	skipTLSVerify  bool          // Skip TLS certificate verification
}

// FTPClientOption is a function that configures FTPClient
type FTPClientOption func(*FTPClient)

// WithFTPTimeout sets the FTP client timeout
func WithFTPTimeout(timeout time.Duration) FTPClientOption {
	return func(c *FTPClient) {
		c.timeout = timeout
	}
}

// WithFTPAuth sets FTP authentication credentials
func WithFTPAuth(username, password string) FTPClientOption {
	return func(c *FTPClient) {
		c.username = username
		c.password = password
	}
}

// WithFTPPassive sets passive mode (default is passive)
func WithFTPPassive(passive bool) FTPClientOption {
	return func(c *FTPClient) {
		c.passive = passive
	}
}

// WithFTPS enables explicit FTPS (AUTH TLS) on standard port 21
func WithFTPS(useTLS bool) FTPClientOption {
	return func(c *FTPClient) {
		c.useTLS = useTLS
	}
}

// WithFTPSImplicit enables implicit FTPS (direct TLS on port 990)
func WithFTPSImplicit(implicit bool) FTPClientOption {
	return func(c *FTPClient) {
		c.implicitTLS = implicit
		if implicit {
			c.useTLS = true
		}
	}
}

// WithFTPTLSConfig sets custom TLS configuration
func WithFTPTLSConfig(config *tls.Config) FTPClientOption {
	return func(c *FTPClient) {
		c.tlsConfig = config
		if config != nil {
			c.useTLS = true
		}
	}
}

// WithFTPSkipTLSVerify skips TLS certificate verification
func WithFTPSkipTLSVerify(skip bool) FTPClientOption {
	return func(c *FTPClient) {
		c.skipTLSVerify = skip
	}
}

// NewFTPClient creates a new FTP client with the given options
func NewFTPClient(opts ...FTPClientOption) *FTPClient {
	c := &FTPClient{
		timeout:  30 * time.Second,
		username: "anonymous",
		password: "burkut@example.com",
		passive:  true,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Supports checks if the URL is supported by this protocol
func (c *FTPClient) Supports(u *url.URL) bool {
	scheme := strings.ToLower(u.Scheme)
	return scheme == "ftp" || scheme == "ftps"
}

// connect establishes a connection to the FTP server
func (c *FTPClient) connect(ctx context.Context, rawURL string) (*ftp.ServerConn, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("parsing URL: %w", err)
	}

	// Check if ftps:// scheme is used
	useTLS := c.useTLS
	implicitTLS := c.implicitTLS
	if strings.ToLower(parsed.Scheme) == "ftps" {
		useTLS = true
	}

	// Determine host:port
	host := parsed.Host
	if !strings.Contains(host, ":") {
		if implicitTLS {
			host = host + ":990" // Implicit FTPS default port
		} else {
			host = host + ":21" // Standard FTP/Explicit FTPS port
		}
	}

	// Build dial options
	dialOpts := []ftp.DialOption{
		ftp.DialWithTimeout(c.timeout),
	}

	// Add TLS options if needed
	if useTLS {
		tlsConfig := c.tlsConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: c.skipTLSVerify,
				ServerName:         parsed.Hostname(),
			}
		}

		if implicitTLS {
			// Implicit TLS: connection starts with TLS
			dialOpts = append(dialOpts, ftp.DialWithTLS(tlsConfig))
		} else {
			// Explicit TLS: upgrade connection with AUTH TLS
			dialOpts = append(dialOpts, ftp.DialWithExplicitTLS(tlsConfig))
		}
	}

	// Connect
	conn, err := ftp.Dial(host, dialOpts...)
	if err != nil {
		return nil, "", fmt.Errorf("connecting to FTP server: %w", err)
	}

	// Get credentials from URL or use configured defaults
	username := c.username
	password := c.password

	if parsed.User != nil {
		username = parsed.User.Username()
		if p, ok := parsed.User.Password(); ok {
			password = p
		}
	}

	// Login
	if err := conn.Login(username, password); err != nil {
		conn.Quit()
		return nil, "", fmt.Errorf("FTP login failed: %w", err)
	}

	// Get the file path
	filepath := parsed.Path
	if filepath == "" {
		filepath = "/"
	}

	return conn, filepath, nil
}

// Head fetches metadata about the file without downloading it
func (c *FTPClient) Head(ctx context.Context, rawURL string) (*Metadata, error) {
	conn, filepath, err := c.connect(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer conn.Quit()

	// Get file size
	size, err := conn.FileSize(filepath)
	if err != nil {
		return nil, fmt.Errorf("getting file size: %w", err)
	}

	// Get file modification time
	modTime, err := conn.GetTime(filepath)
	if err != nil {
		// Non-fatal, some servers don't support MDTM
		modTime = time.Time{}
	}

	// Extract filename from path
	filename := path.Base(filepath)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	return &Metadata{
		URL:           rawURL,
		Filename:      filename,
		ContentLength: size,
		AcceptRanges:  true, // FTP supports REST command for resume
		ContentType:   "application/octet-stream",
		LastModified:  modTime,
	}, nil
}

// Get downloads the entire file
func (c *FTPClient) Get(ctx context.Context, rawURL string) (io.ReadCloser, *Metadata, error) {
	conn, filepath, err := c.connect(ctx, rawURL)
	if err != nil {
		return nil, nil, err
	}

	// Get metadata first
	size, err := conn.FileSize(filepath)
	if err != nil {
		conn.Quit()
		return nil, nil, fmt.Errorf("getting file size: %w", err)
	}

	// Retrieve file
	resp, err := conn.Retr(filepath)
	if err != nil {
		conn.Quit()
		return nil, nil, fmt.Errorf("retrieving file: %w", err)
	}

	// Extract filename
	filename := path.Base(filepath)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	meta := &Metadata{
		URL:           rawURL,
		Filename:      filename,
		ContentLength: size,
		AcceptRanges:  true,
		ContentType:   "application/octet-stream",
	}

	// Wrap response to close connection when done
	return &ftpReadCloser{
		ReadCloser: resp,
		conn:       conn,
	}, meta, nil
}

// GetRange downloads a specific byte range of the file
func (c *FTPClient) GetRange(ctx context.Context, rawURL string, start, end int64) (io.ReadCloser, error) {
	conn, filepath, err := c.connect(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	// Get file size for validation
	size, err := conn.FileSize(filepath)
	if err != nil {
		conn.Quit()
		return nil, fmt.Errorf("getting file size: %w", err)
	}

	// Validate range
	if start >= size {
		conn.Quit()
		return nil, fmt.Errorf("start offset %d exceeds file size %d", start, size)
	}

	// Use REST command to set the starting offset
	resp, err := conn.RetrFrom(filepath, uint64(start))
	if err != nil {
		conn.Quit()
		return nil, fmt.Errorf("retrieving file from offset: %w", err)
	}

	// Calculate bytes to read (end is inclusive)
	bytesToRead := end - start + 1

	// Wrap with limit reader and connection closer
	return &ftpRangeReader{
		reader:      io.LimitReader(resp, bytesToRead),
		resp:        resp,
		conn:        conn,
		bytesToRead: bytesToRead,
	}, nil
}

// ftpReadCloser wraps FTP response to close connection when done
type ftpReadCloser struct {
	io.ReadCloser
	conn *ftp.ServerConn
}

func (f *ftpReadCloser) Close() error {
	err := f.ReadCloser.Close()
	f.conn.Quit()
	return err
}

// ftpRangeReader handles FTP range requests
type ftpRangeReader struct {
	reader      io.Reader
	resp        *ftp.Response
	conn        *ftp.ServerConn
	bytesToRead int64
}

func (f *ftpRangeReader) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func (f *ftpRangeReader) Close() error {
	f.resp.Close()
	f.conn.Quit()
	return nil
}
