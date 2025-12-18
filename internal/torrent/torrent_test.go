package torrent

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DownloadDir != "." {
		t.Errorf("DownloadDir = %q, want \".\"", cfg.DownloadDir)
	}

	if cfg.MaxConnections != 50 {
		t.Errorf("MaxConnections = %d, want 50", cfg.MaxConnections)
	}

	if cfg.Seed != false {
		t.Error("Seed should be false by default")
	}

	if cfg.NoDHT != false {
		t.Error("NoDHT should be false by default")
	}

	if cfg.EnableUPnP != true {
		t.Error("EnableUPnP should be true by default")
	}
}

func TestIsMagnetURI(t *testing.T) {
	tests := []struct {
		uri  string
		want bool
	}{
		{"magnet:?xt=urn:btih:abc123", true},
		{"magnet:?xt=urn:btih:ABC123&dn=filename", true},
		{"http://example.com/file.torrent", false},
		{"https://example.com/file.torrent", false},
		{"file.torrent", false},
		{"magnet", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := IsMagnetURI(tt.uri)
			if got != tt.want {
				t.Errorf("IsMagnetURI(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestIsTorrentFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"file.torrent", true},
		{"path/to/file.torrent", true},
		{"/absolute/path/file.torrent", true},
		{"file.TORRENT", false}, // Case sensitive
		{"file.txt", false},
		{"file", false},
		{"torrent", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsTorrentFile(tt.path)
			if got != tt.want {
				t.Errorf("IsTorrentFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDownloadStatus_String(t *testing.T) {
	tests := []struct {
		status DownloadStatus
		want   string
	}{
		{StatusPending, "pending"},
		{StatusDownloading, "downloading"},
		{StatusSeeding, "seeding"},
		{StatusPaused, "paused"},
		{StatusCompleted, "completed"},
		{StatusError, "error"},
		{DownloadStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.want {
				t.Errorf("DownloadStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
