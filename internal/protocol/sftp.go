// Package protocol provides protocol adapters for different download protocols.
package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPClient is an SFTP protocol adapter for downloading files
type SFTPClient struct {
	timeout     time.Duration
	username    string
	password    string
	privateKey  string
	knownHosts  string
	insecure    bool // Skip host key verification
}

// SFTPClientOption is a function that configures SFTPClient
type SFTPClientOption func(*SFTPClient)

// WithSFTPTimeout sets the SFTP client timeout
func WithSFTPTimeout(timeout time.Duration) SFTPClientOption {
	return func(c *SFTPClient) {
		c.timeout = timeout
	}
}

// WithSFTPAuth sets SFTP password authentication
func WithSFTPAuth(username, password string) SFTPClientOption {
	return func(c *SFTPClient) {
		c.username = username
		c.password = password
	}
}

// WithSFTPPrivateKey sets SFTP private key authentication
func WithSFTPPrivateKey(username, keyPath string) SFTPClientOption {
	return func(c *SFTPClient) {
		c.username = username
		c.privateKey = keyPath
	}
}

// WithSFTPInsecure skips host key verification
func WithSFTPInsecure(insecure bool) SFTPClientOption {
	return func(c *SFTPClient) {
		c.insecure = insecure
	}
}

// NewSFTPClient creates a new SFTP client with the given options
func NewSFTPClient(opts ...SFTPClientOption) *SFTPClient {
	c := &SFTPClient{
		timeout:  30 * time.Second,
		insecure: true, // Default to insecure for simplicity
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Supports checks if the URL is supported by this protocol
func (c *SFTPClient) Supports(u *url.URL) bool {
	scheme := strings.ToLower(u.Scheme)
	return scheme == "sftp"
}

// connect establishes a connection to the SFTP server
func (c *SFTPClient) connect(ctx context.Context, rawURL string) (*ssh.Client, *sftp.Client, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, "", fmt.Errorf("parsing URL: %w", err)
	}

	// Determine host:port
	host := parsed.Host
	if !strings.Contains(host, ":") {
		host = host + ":22"
	}

	// Get credentials from URL or configured defaults
	username := c.username
	password := c.password

	if parsed.User != nil {
		username = parsed.User.Username()
		if p, ok := parsed.User.Password(); ok {
			password = p
		}
	}

	if username == "" {
		username = os.Getenv("USER")
		if username == "" {
			username = os.Getenv("USERNAME") // Windows
		}
	}

	// Build auth methods
	var authMethods []ssh.AuthMethod

	// Try private key first
	if c.privateKey != "" {
		key, err := loadPrivateKey(c.privateKey)
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(key))
		}
	} else {
		// Try default SSH keys
		homeDir, _ := os.UserHomeDir()
		keyPaths := []string{
			filepath.Join(homeDir, ".ssh", "id_rsa"),
			filepath.Join(homeDir, ".ssh", "id_ed25519"),
			filepath.Join(homeDir, ".ssh", "id_ecdsa"),
		}
		for _, keyPath := range keyPaths {
			if key, err := loadPrivateKey(keyPath); err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(key))
				break
			}
		}
	}

	// Add password auth if available
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}

	if len(authMethods) == 0 {
		return nil, nil, "", fmt.Errorf("no authentication method available")
	}

	// Build SSH config
	sshConfig := &ssh.ClientConfig{
		User:    username,
		Auth:    authMethods,
		Timeout: c.timeout,
	}

	if c.insecure {
		sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey() // TODO: Implement known_hosts
	}

	// Connect SSH
	sshConn, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		return nil, nil, "", fmt.Errorf("SSH connection failed: %w", err)
	}

	// Create SFTP session
	sftpClient, err := sftp.NewClient(sshConn)
	if err != nil {
		sshConn.Close()
		return nil, nil, "", fmt.Errorf("SFTP session failed: %w", err)
	}

	// Get the file path
	filepath := parsed.Path
	if filepath == "" {
		filepath = "/"
	}

	return sshConn, sftpClient, filepath, nil
}

// loadPrivateKey loads an SSH private key from file
func loadPrivateKey(keyPath string) (ssh.Signer, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	return signer, nil
}

// Head fetches metadata about the file without downloading it
func (c *SFTPClient) Head(ctx context.Context, rawURL string) (*Metadata, error) {
	sshConn, sftpClient, filepath, err := c.connect(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer sftpClient.Close()
	defer sshConn.Close()

	// Get file info
	info, err := sftpClient.Stat(filepath)
	if err != nil {
		return nil, fmt.Errorf("getting file info: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}

	// Extract filename from path
	filename := path.Base(filepath)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	return &Metadata{
		URL:           rawURL,
		Filename:      filename,
		ContentLength: info.Size(),
		AcceptRanges:  true, // SFTP supports seek
		ContentType:   "application/octet-stream",
		LastModified:  info.ModTime(),
	}, nil
}

// Get downloads the entire file
func (c *SFTPClient) Get(ctx context.Context, rawURL string) (io.ReadCloser, *Metadata, error) {
	sshConn, sftpClient, filepath, err := c.connect(ctx, rawURL)
	if err != nil {
		return nil, nil, err
	}

	// Get file info
	info, err := sftpClient.Stat(filepath)
	if err != nil {
		sftpClient.Close()
		sshConn.Close()
		return nil, nil, fmt.Errorf("getting file info: %w", err)
	}

	// Open file
	file, err := sftpClient.Open(filepath)
	if err != nil {
		sftpClient.Close()
		sshConn.Close()
		return nil, nil, fmt.Errorf("opening file: %w", err)
	}

	// Extract filename
	filename := path.Base(filepath)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	meta := &Metadata{
		URL:           rawURL,
		Filename:      filename,
		ContentLength: info.Size(),
		AcceptRanges:  true,
		ContentType:   "application/octet-stream",
		LastModified:  info.ModTime(),
	}

	return &sftpReadCloser{
		file:       file,
		sftpClient: sftpClient,
		sshConn:    sshConn,
	}, meta, nil
}

// GetRange downloads a specific byte range of the file
func (c *SFTPClient) GetRange(ctx context.Context, rawURL string, start, end int64) (io.ReadCloser, error) {
	sshConn, sftpClient, filepath, err := c.connect(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	// Get file info
	info, err := sftpClient.Stat(filepath)
	if err != nil {
		sftpClient.Close()
		sshConn.Close()
		return nil, fmt.Errorf("getting file info: %w", err)
	}

	// Validate range
	if start >= info.Size() {
		sftpClient.Close()
		sshConn.Close()
		return nil, fmt.Errorf("start offset %d exceeds file size %d", start, info.Size())
	}

	// Open file
	file, err := sftpClient.Open(filepath)
	if err != nil {
		sftpClient.Close()
		sshConn.Close()
		return nil, fmt.Errorf("opening file: %w", err)
	}

	// Seek to start position
	_, err = file.Seek(start, io.SeekStart)
	if err != nil {
		file.Close()
		sftpClient.Close()
		sshConn.Close()
		return nil, fmt.Errorf("seeking to position: %w", err)
	}

	// Calculate bytes to read
	bytesToRead := end - start + 1

	return &sftpRangeReader{
		file:        file,
		reader:      io.LimitReader(file, bytesToRead),
		sftpClient:  sftpClient,
		sshConn:     sshConn,
	}, nil
}

// sftpReadCloser wraps SFTP file to close connections when done
type sftpReadCloser struct {
	file       *sftp.File
	sftpClient *sftp.Client
	sshConn    *ssh.Client
}

func (s *sftpReadCloser) Read(p []byte) (int, error) {
	return s.file.Read(p)
}

func (s *sftpReadCloser) Close() error {
	s.file.Close()
	s.sftpClient.Close()
	s.sshConn.Close()
	return nil
}

// sftpRangeReader handles SFTP range requests
type sftpRangeReader struct {
	file       *sftp.File
	reader     io.Reader
	sftpClient *sftp.Client
	sshConn    *ssh.Client
}

func (s *sftpRangeReader) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *sftpRangeReader) Close() error {
	s.file.Close()
	s.sftpClient.Close()
	s.sshConn.Close()
	return nil
}

// checkHostKey returns a callback that verifies the host key
// TODO: Implement proper known_hosts checking
func checkHostKey(knownHostsFile string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// For now, just accept all keys
		return nil
	}
}
