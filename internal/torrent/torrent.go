// Package torrent provides BitTorrent protocol support using anacrolix/torrent.
package torrent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"golang.org/x/time/rate"
)

// Client wraps anacrolix/torrent client with Burkut-specific functionality
type Client struct {
	client    *torrent.Client
	config    *Config
	mu        sync.Mutex
	downloads map[string]*Download
}

// Config holds torrent client configuration
type Config struct {
	DownloadDir     string        // Directory for downloaded files
	DataDir         string        // Directory for torrent data/cache
	MaxConnections  int           // Maximum connections per torrent
	UploadLimit     int64         // Upload speed limit (bytes/s), 0 = unlimited
	DownloadLimit   int64         // Download speed limit (bytes/s), 0 = unlimited
	Seed            bool          // Continue seeding after download
	SeedRatio       float64       // Stop seeding after this ratio (0 = unlimited)
	NoDHT           bool          // Disable DHT
	NoPEX           bool          // Disable PEX (Peer Exchange)
	ListenPort      int           // Port to listen on (0 = random)
	EnableUPnP      bool          // Enable UPnP port forwarding
	UserAgent       string        // Client user agent
	ConnectTimeout  time.Duration // Peer connect timeout
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() *Config {
	return &Config{
		DownloadDir:    ".",
		DataDir:        "",
		MaxConnections: 50,
		UploadLimit:    0,
		DownloadLimit:  0,
		Seed:           false,
		SeedRatio:      0,
		NoDHT:          false,
		NoPEX:          false,
		ListenPort:     0,
		EnableUPnP:     true,
		UserAgent:      "Burkut/1.0",
		ConnectTimeout: 30 * time.Second,
	}
}

// Download represents an active torrent download
type Download struct {
	Torrent    *torrent.Torrent
	Name       string
	InfoHash   string
	TotalSize  int64
	Downloaded int64
	Uploaded   int64
	Progress   float64
	Speed      int64
	Peers      int
	Seeds      int
	Status     DownloadStatus
	Error      error
	StartTime  time.Time
}

// DownloadStatus represents the current state of a download
type DownloadStatus int

const (
	StatusPending DownloadStatus = iota
	StatusDownloading
	StatusSeeding
	StatusPaused
	StatusCompleted
	StatusError
)

func (s DownloadStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusDownloading:
		return "downloading"
	case StatusSeeding:
		return "seeding"
	case StatusPaused:
		return "paused"
	case StatusCompleted:
		return "completed"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// Progress holds download progress information
type Progress struct {
	Name       string
	InfoHash   string
	TotalSize  int64
	Downloaded int64
	Uploaded   int64
	Percent    float64
	Speed      int64
	ETA        time.Duration
	Peers      int
	Seeds      int
	Status     DownloadStatus
}

// NewClient creates a new torrent client
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Create torrent client config
	clientCfg := torrent.NewDefaultClientConfig()
	clientCfg.DataDir = cfg.DownloadDir
	clientCfg.NoDHT = cfg.NoDHT
	clientCfg.DisablePEX = cfg.NoPEX

	if cfg.ListenPort > 0 {
		clientCfg.ListenPort = cfg.ListenPort
	}

	// Set rate limits
	if cfg.DownloadLimit > 0 {
		clientCfg.DownloadRateLimiter = rate.NewLimiter(rate.Limit(cfg.DownloadLimit), int(cfg.DownloadLimit))
	}
	if cfg.UploadLimit > 0 {
		clientCfg.UploadRateLimiter = rate.NewLimiter(rate.Limit(cfg.UploadLimit), int(cfg.UploadLimit))
	}

	// Create client
	client, err := torrent.NewClient(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create torrent client: %w", err)
	}

	return &Client{
		client:    client,
		config:    cfg,
		downloads: make(map[string]*Download),
	}, nil
}

// AddMagnet adds a torrent from a magnet link
func (c *Client) AddMagnet(magnetURI string) (*Download, error) {
	t, err := c.client.AddMagnet(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed to add magnet: %w", err)
	}

	// Wait for metadata
	<-t.GotInfo()

	return c.createDownload(t), nil
}

// AddTorrentFile adds a torrent from a .torrent file
func (c *Client) AddTorrentFile(path string) (*Download, error) {
	mi, err := metainfo.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load torrent file: %w", err)
	}

	t, err := c.client.AddTorrent(mi)
	if err != nil {
		return nil, fmt.Errorf("failed to add torrent: %w", err)
	}

	<-t.GotInfo()

	return c.createDownload(t), nil
}

// AddTorrentReader adds a torrent from an io.Reader
func (c *Client) AddTorrentReader(r io.Reader) (*Download, error) {
	mi, err := metainfo.Load(r)
	if err != nil {
		return nil, fmt.Errorf("failed to load torrent: %w", err)
	}

	t, err := c.client.AddTorrent(mi)
	if err != nil {
		return nil, fmt.Errorf("failed to add torrent: %w", err)
	}

	<-t.GotInfo()

	return c.createDownload(t), nil
}

func (c *Client) createDownload(t *torrent.Torrent) *Download {
	info := t.Info()
	d := &Download{
		Torrent:   t,
		Name:      t.Name(),
		InfoHash:  t.InfoHash().HexString(),
		TotalSize: info.TotalLength(),
		Status:    StatusPending,
		StartTime: time.Now(),
	}

	c.mu.Lock()
	c.downloads[d.InfoHash] = d
	c.mu.Unlock()

	return d
}

// Download starts downloading a torrent and blocks until complete or context cancelled
func (c *Client) Download(ctx context.Context, d *Download, progressCb func(Progress)) error {
	t := d.Torrent

	// Download all files
	t.DownloadAll()
	d.Status = StatusDownloading

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastBytes int64
	lastTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			stats := t.Stats()
			bytesCompleted := t.BytesCompleted()

			// Calculate speed
			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()
			if elapsed > 0 {
				d.Speed = int64(float64(bytesCompleted-lastBytes) / elapsed)
			}
			lastBytes = bytesCompleted
			lastTime = now

			// Update download info
			d.Downloaded = bytesCompleted
			d.Progress = float64(bytesCompleted) / float64(d.TotalSize) * 100
			d.Peers = stats.TotalPeers
			d.Seeds = stats.ConnectedSeeders

			// Calculate ETA
			var eta time.Duration
			if d.Speed > 0 {
				remaining := d.TotalSize - d.Downloaded
				eta = time.Duration(remaining/d.Speed) * time.Second
			}

			// Report progress
			if progressCb != nil {
				progressCb(Progress{
					Name:       d.Name,
					InfoHash:   d.InfoHash,
					TotalSize:  d.TotalSize,
					Downloaded: d.Downloaded,
					Uploaded:   d.Uploaded,
					Percent:    d.Progress,
					Speed:      d.Speed,
					ETA:        eta,
					Peers:      d.Peers,
					Seeds:      d.Seeds,
					Status:     d.Status,
				})
			}

			// Check completion
			if bytesCompleted >= d.TotalSize {
				d.Status = StatusCompleted
				return nil
			}
		}
	}
}

// GetDownload returns a download by info hash
func (c *Client) GetDownload(infoHash string) *Download {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.downloads[infoHash]
}

// ListDownloads returns all downloads
func (c *Client) ListDownloads() []*Download {
	c.mu.Lock()
	defer c.mu.Unlock()

	downloads := make([]*Download, 0, len(c.downloads))
	for _, d := range c.downloads {
		downloads = append(downloads, d)
	}
	return downloads
}

// Pause pauses a download
func (c *Client) Pause(d *Download) {
	d.Torrent.DisallowDataDownload()
	d.Status = StatusPaused
}

// Resume resumes a paused download
func (c *Client) Resume(d *Download) {
	d.Torrent.AllowDataDownload()
	d.Status = StatusDownloading
}

// Remove removes a download (optionally delete files)
func (c *Client) Remove(d *Download, deleteFiles bool) error {
	d.Torrent.Drop()

	c.mu.Lock()
	delete(c.downloads, d.InfoHash)
	c.mu.Unlock()

	if deleteFiles {
		// Delete downloaded files
		info := d.Torrent.Info()
		if info != nil {
			for _, file := range info.Files {
				path := filepath.Join(c.config.DownloadDir, file.DisplayPath(info))
				os.Remove(path)
			}
		}
	}

	return nil
}

// Close closes the torrent client
func (c *Client) Close() error {
	errs := c.client.Close()
	if len(errs) > 0 {
		return fmt.Errorf("errors closing client: %v", errs)
	}
	return nil
}

// IsMagnetURI checks if a string is a magnet URI
func IsMagnetURI(uri string) bool {
	return len(uri) > 8 && uri[:8] == "magnet:?"
}

// IsTorrentFile checks if a path is a .torrent file
func IsTorrentFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".torrent"
}
