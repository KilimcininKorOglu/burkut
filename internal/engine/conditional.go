// Package engine provides the core download engine with parallel chunk support.
package engine

import (
	"fmt"
	"os"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/protocol"
)

// ConditionalCheck represents the result of a conditional download check
type ConditionalCheck struct {
	ShouldDownload bool
	Reason         string
	LocalModTime   time.Time
	RemoteModTime  time.Time
	LocalSize      int64
	RemoteSize     int64
	LocalETag      string
	RemoteETag     string
}

// CheckTimestamp checks if download is needed based on file modification times
// Returns true if download should proceed, false if local file is up-to-date
func CheckTimestamp(localPath string, remoteMeta *protocol.Metadata) (*ConditionalCheck, error) {
	result := &ConditionalCheck{
		ShouldDownload: true,
		RemoteModTime:  remoteMeta.LastModified,
		RemoteSize:     remoteMeta.ContentLength,
		RemoteETag:     remoteMeta.ETag,
	}

	// Check if local file exists
	info, err := os.Stat(localPath)
	if os.IsNotExist(err) {
		result.Reason = "local file does not exist"
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("checking local file: %w", err)
	}

	result.LocalModTime = info.ModTime()
	result.LocalSize = info.Size()

	// If remote server didn't provide Last-Modified, we can't compare
	if remoteMeta.LastModified.IsZero() {
		result.Reason = "server did not provide Last-Modified header"
		return result, nil
	}

	// Compare modification times
	// Local file is considered up-to-date if it's newer or same as remote
	if !result.LocalModTime.Before(remoteMeta.LastModified) {
		// Also check size if available
		if remoteMeta.ContentLength > 0 && result.LocalSize == remoteMeta.ContentLength {
			result.ShouldDownload = false
			result.Reason = "local file is up-to-date (same or newer modification time and size)"
			return result, nil
		} else if remoteMeta.ContentLength <= 0 {
			// Size unknown, rely on timestamp only
			result.ShouldDownload = false
			result.Reason = "local file is up-to-date (same or newer modification time)"
			return result, nil
		}
		// Size mismatch, download anyway
		result.Reason = "local file has different size than remote"
		return result, nil
	}

	result.Reason = "remote file is newer than local file"
	return result, nil
}

// CheckETag checks if download is needed based on ETag comparison
// Returns true if download should proceed, false if local file matches remote ETag
func CheckETag(localPath string, localETag string, remoteMeta *protocol.Metadata) (*ConditionalCheck, error) {
	result := &ConditionalCheck{
		ShouldDownload: true,
		RemoteModTime:  remoteMeta.LastModified,
		RemoteSize:     remoteMeta.ContentLength,
		RemoteETag:     remoteMeta.ETag,
		LocalETag:      localETag,
	}

	// Check if local file exists
	info, err := os.Stat(localPath)
	if os.IsNotExist(err) {
		result.Reason = "local file does not exist"
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("checking local file: %w", err)
	}

	result.LocalModTime = info.ModTime()
	result.LocalSize = info.Size()

	// If remote server didn't provide ETag, we can't compare
	if remoteMeta.ETag == "" {
		result.Reason = "server did not provide ETag header"
		return result, nil
	}

	// If no local ETag provided, we can't compare
	if localETag == "" {
		result.Reason = "no local ETag available for comparison"
		return result, nil
	}

	// Compare ETags (normalize by removing quotes if present)
	remoteETag := normalizeETag(remoteMeta.ETag)
	localETagNorm := normalizeETag(localETag)

	if remoteETag == localETagNorm {
		result.ShouldDownload = false
		result.Reason = "local file matches remote ETag"
		return result, nil
	}

	result.Reason = "ETag mismatch, remote file has changed"
	return result, nil
}

// normalizeETag removes surrounding quotes from ETag value
func normalizeETag(etag string) string {
	if len(etag) >= 2 {
		if (etag[0] == '"' && etag[len(etag)-1] == '"') ||
			(etag[0] == '\'' && etag[len(etag)-1] == '\'') {
			return etag[1 : len(etag)-1]
		}
		// Handle weak ETags (W/"...")
		if len(etag) >= 4 && etag[0:2] == "W/" {
			return normalizeETag(etag[2:])
		}
	}
	return etag
}

// SetFileModTime sets the modification time of a file to match the remote file
func SetFileModTime(localPath string, modTime time.Time) error {
	if modTime.IsZero() {
		return nil // Nothing to set
	}
	return os.Chtimes(localPath, modTime, modTime)
}

// ETagStore manages ETag storage for downloaded files
type ETagStore struct {
	storePath string
	etags     map[string]string // URL -> ETag mapping
}

// NewETagStore creates a new ETag store
func NewETagStore(storePath string) *ETagStore {
	return &ETagStore{
		storePath: storePath,
		etags:     make(map[string]string),
	}
}

// Get retrieves the stored ETag for a URL
func (s *ETagStore) Get(url string) string {
	return s.etags[url]
}

// Set stores an ETag for a URL
func (s *ETagStore) Set(url string, etag string) {
	s.etags[url] = etag
}

// Delete removes an ETag for a URL
func (s *ETagStore) Delete(url string) {
	delete(s.etags, url)
}
