// Package engine provides the core download engine with parallel chunk support.
package engine

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/download"
	"github.com/kilimcininkoroglu/burkut/internal/protocol"
	"github.com/kilimcininkoroglu/burkut/internal/storage"
)

// Progress represents the current download progress
type Progress struct {
	Downloaded   int64
	TotalSize    int64
	Speed        int64 // bytes per second
	Percent      float64
	ChunkStatus  []ChunkProgress
	StartTime    time.Time
	ElapsedTime  time.Duration
	RemainingETA time.Duration
}

// ChunkProgress represents progress of a single chunk
type ChunkProgress struct {
	ID         int
	Downloaded int64
	Total      int64
	Speed      int64
	Status     download.ChunkStatus
}

// ProgressCallback is called periodically with progress updates
type ProgressCallback func(Progress)

// DownloaderConfig holds configuration for the downloader
type DownloaderConfig struct {
	Connections     int
	BufferSize      int
	ProgressInterval time.Duration
	SaveInterval    time.Duration
}

// DefaultConfig returns default downloader configuration
func DefaultConfig() DownloaderConfig {
	return DownloaderConfig{
		Connections:     4,
		BufferSize:      32 * 1024, // 32KB buffer
		ProgressInterval: 100 * time.Millisecond,
		SaveInterval:    5 * time.Second,
	}
}

// Downloader manages the download of a single file
type Downloader struct {
	config     DownloaderConfig
	httpClient *protocol.HTTPClient
	state      *download.State
	writer     *storage.FileWriter
	outputPath string

	// Progress tracking
	downloaded   int64
	startTime    time.Time
	lastBytes    int64
	lastTime     time.Time
	speedSamples []int64
	progressCB   ProgressCallback

	// Synchronization
	mu       sync.RWMutex
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	doneChan chan struct{}
}

// NewDownloader creates a new Downloader
func NewDownloader(config DownloaderConfig, httpClient *protocol.HTTPClient) *Downloader {
	return &Downloader{
		config:       config,
		httpClient:   httpClient,
		speedSamples: make([]int64, 0, 10),
		doneChan:     make(chan struct{}),
	}
}

// SetProgressCallback sets the progress callback function
func (d *Downloader) SetProgressCallback(cb ProgressCallback) {
	d.progressCB = cb
}

// Download starts downloading from the given URL to the output path
func (d *Downloader) Download(ctx context.Context, url, outputPath string) error {
	d.outputPath = outputPath

	// Create cancellable context
	ctx, d.cancel = context.WithCancel(ctx)
	defer d.cancel()

	// Get file metadata
	meta, err := d.httpClient.Head(ctx, url)
	if err != nil {
		return fmt.Errorf("getting file metadata: %w", err)
	}

	// Check for existing state (resume)
	if download.StateExists(outputPath) {
		d.state, err = download.LoadState(outputPath)
		if err != nil {
			// Corrupted state, start fresh
			d.state = nil
		} else if d.state.URL != url || d.state.TotalSize != meta.ContentLength {
			// Different file, start fresh
			d.state = nil
		}
	}

	// Create new state if needed
	if d.state == nil {
		d.state = download.NewState(url, meta.Filename, meta.ContentLength, meta.AcceptRanges)

		// Initialize chunks
		numChunks := d.config.Connections
		if !meta.AcceptRanges || meta.ContentLength <= 0 {
			numChunks = 1 // Single chunk for non-resumable downloads
		}
		d.state.InitializeChunks(numChunks)
	}

	// Create or open file writer
	if storage.FileExists(outputPath) && d.state.Downloaded > 0 {
		d.writer, err = storage.OpenFileWriter(outputPath, meta.ContentLength)
	} else {
		d.writer, err = storage.NewFileWriter(outputPath, meta.ContentLength)
	}
	if err != nil {
		return fmt.Errorf("creating file writer: %w", err)
	}
	defer d.writer.Close()

	// Record start time
	d.startTime = time.Now()
	d.lastTime = d.startTime
	d.downloaded = d.state.Downloaded

	// Start progress reporter
	go d.progressReporter(ctx)

	// Start state saver
	go d.stateSaver(ctx)

	// Download chunks
	if err := d.downloadChunks(ctx, url); err != nil {
		// Save state on error for resume
		d.state.Save(outputPath)
		return err
	}

	// Download complete - remove state file
	download.DeleteState(outputPath)

	// Truncate file to exact size if known
	if meta.ContentLength > 0 {
		d.writer.Truncate(meta.ContentLength)
	}

	close(d.doneChan)
	return nil
}

// downloadChunks downloads all chunks in parallel
func (d *Downloader) downloadChunks(ctx context.Context, url string) error {
	pendingChunks := d.state.GetPendingChunks()
	if len(pendingChunks) == 0 {
		return nil // Already complete
	}

	errChan := make(chan error, len(pendingChunks))
	semaphore := make(chan struct{}, d.config.Connections)

	for _, chunk := range pendingChunks {
		d.wg.Add(1)
		go func(c download.Chunk) {
			defer d.wg.Done()

			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			if err := d.downloadChunk(ctx, url, c); err != nil {
				select {
				case errChan <- fmt.Errorf("chunk %d: %w", c.ID, err):
				default:
				}
			}
		}(chunk)
	}

	// Wait for all chunks
	d.wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return err
	}

	return nil
}

// downloadChunk downloads a single chunk
func (d *Downloader) downloadChunk(ctx context.Context, url string, chunk download.Chunk) error {
	// Mark chunk as in progress
	d.state.UpdateChunk(chunk.ID, chunk.Downloaded, download.ChunkStatusInProgress)

	var reader io.ReadCloser
	var err error

	// Calculate byte range
	start := chunk.CurrentPosition()
	end := chunk.End

	if end < 0 {
		// Unknown size - download everything
		reader, _, err = d.httpClient.Get(ctx, url)
	} else if start > end {
		// Chunk already complete
		d.state.UpdateChunk(chunk.ID, chunk.Size(), download.ChunkStatusCompleted)
		return nil
	} else {
		// Range request
		reader, err = d.httpClient.GetRange(ctx, url, start, end)
	}

	if err != nil {
		d.state.UpdateChunk(chunk.ID, chunk.Downloaded, download.ChunkStatusFailed)
		return err
	}
	defer reader.Close()

	// Download with buffer
	buffer := make([]byte, d.config.BufferSize)
	offset := start
	downloaded := chunk.Downloaded

	for {
		select {
		case <-ctx.Done():
			d.state.UpdateChunk(chunk.ID, downloaded, download.ChunkStatusPending)
			return ctx.Err()
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			// Write to file
			if _, writeErr := d.writer.WriteAt(buffer[:n], offset); writeErr != nil {
				d.state.UpdateChunk(chunk.ID, downloaded, download.ChunkStatusFailed)
				return writeErr
			}

			offset += int64(n)
			downloaded += int64(n)

			// Update progress
			atomic.AddInt64(&d.downloaded, int64(n))
			d.state.UpdateChunk(chunk.ID, downloaded, download.ChunkStatusInProgress)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			d.state.UpdateChunk(chunk.ID, downloaded, download.ChunkStatusFailed)
			return err
		}
	}

	// Mark chunk complete
	d.state.UpdateChunk(chunk.ID, downloaded, download.ChunkStatusCompleted)
	return nil
}

// progressReporter periodically reports progress
func (d *Downloader) progressReporter(ctx context.Context) {
	ticker := time.NewTicker(d.config.ProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.doneChan:
			return
		case <-ticker.C:
			if d.progressCB != nil {
				d.progressCB(d.GetProgress())
			}
		}
	}
}

// stateSaver periodically saves state
func (d *Downloader) stateSaver(ctx context.Context) {
	ticker := time.NewTicker(d.config.SaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.doneChan:
			return
		case <-ticker.C:
			d.state.Save(d.outputPath)
		}
	}
}

// GetProgress returns current download progress
func (d *Downloader) GetProgress() Progress {
	d.mu.RLock()
	defer d.mu.RUnlock()

	now := time.Now()
	currentBytes := atomic.LoadInt64(&d.downloaded)
	elapsed := now.Sub(d.startTime)

	// Calculate speed (bytes per second)
	var speed int64
	if elapsed > 0 {
		deltaBytes := currentBytes - d.lastBytes
		deltaTime := now.Sub(d.lastTime)
		if deltaTime > 0 {
			instantSpeed := float64(deltaBytes) / deltaTime.Seconds()
			
			// Add to samples for smoothing
			if len(d.speedSamples) >= 10 {
				d.speedSamples = d.speedSamples[1:]
			}
			d.speedSamples = append(d.speedSamples, int64(instantSpeed))
			
			// Calculate average speed
			var total int64
			for _, s := range d.speedSamples {
				total += s
			}
			speed = total / int64(len(d.speedSamples))
		}
	}

	// Update last values (need write lock for this, but we're keeping it simple)
	d.lastBytes = currentBytes
	d.lastTime = now

	// Calculate ETA
	var eta time.Duration
	if speed > 0 && d.state.TotalSize > 0 {
		remaining := d.state.TotalSize - currentBytes
		eta = time.Duration(float64(remaining) / float64(speed) * float64(time.Second))
	}

	// Calculate percent
	var percent float64
	if d.state.TotalSize > 0 {
		percent = float64(currentBytes) / float64(d.state.TotalSize) * 100
	}

	// Build chunk progress
	chunkProgress := make([]ChunkProgress, len(d.state.Chunks))
	for i, chunk := range d.state.Chunks {
		chunkProgress[i] = ChunkProgress{
			ID:         chunk.ID,
			Downloaded: chunk.Downloaded,
			Total:      chunk.Size(),
			Status:     chunk.Status,
		}
	}

	return Progress{
		Downloaded:   currentBytes,
		TotalSize:    d.state.TotalSize,
		Speed:        speed,
		Percent:      percent,
		ChunkStatus:  chunkProgress,
		StartTime:    d.startTime,
		ElapsedTime:  elapsed,
		RemainingETA: eta,
	}
}

// Cancel cancels the download
func (d *Downloader) Cancel() {
	if d.cancel != nil {
		d.cancel()
	}
}

// State returns the current download state
func (d *Downloader) State() *download.State {
	return d.state
}
