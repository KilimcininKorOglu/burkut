package tui

import (
	"context"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kilimcininkoroglu/burkut/internal/engine"
)

// Runner manages the TUI and download coordination
type Runner struct {
	model    *Model
	program  *tea.Program
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	paused   bool
	pauseCh  chan struct{}
	resumeCh chan struct{}
}

// NewRunner creates a new TUI runner
func NewRunner(url, filename string, totalSize int64, connections int) *Runner {
	ctx, cancel := context.WithCancel(context.Background())

	model := NewModel(url, filename, totalSize, connections)

	r := &Runner{
		model:    &model,
		ctx:      ctx,
		cancel:   cancel,
		pauseCh:  make(chan struct{}),
		resumeCh: make(chan struct{}),
	}

	// Set callbacks
	model.SetCallbacks(r.onPause, r.onResume, r.onCancel)
	r.model = &model

	return r
}

// Start starts the TUI
func (r *Runner) Start() error {
	r.program = tea.NewProgram(r.model, tea.WithAltScreen())

	_, err := r.program.Run()
	return err
}

// StartAsync starts the TUI in a goroutine
func (r *Runner) StartAsync() {
	r.program = tea.NewProgram(r.model, tea.WithAltScreen())
	go r.program.Run()
}

// UpdateProgress sends a progress update to the TUI
func (r *Runner) UpdateProgress(p engine.Progress, chunks []engine.ChunkProgress) {
	if r.program == nil {
		return
	}

	chunkInfos := make([]ChunkInfo, len(chunks))
	for i, c := range chunks {
		status := "pending"
		switch {
		case c.Status == "completed":
			status = "completed"
		case c.Downloaded > 0:
			status = "downloading"
		case c.Status == "error":
			status = "error"
		}

		chunkInfos[i] = ChunkInfo{
			ID:         c.ID,
			Start:      c.Start,
			End:        c.End,
			Downloaded: c.Downloaded,
			Status:     status,
		}
	}

	r.program.Send(ProgressMsg{
		Progress: p,
		Chunks:   chunkInfos,
	})
}

// SetComplete marks the download as complete
func (r *Runner) SetComplete(filename string, size int64, duration time.Duration, speed int64) {
	if r.program == nil {
		return
	}

	r.program.Send(CompleteMsg{
		Filename: filename,
		Size:     size,
		Duration: duration,
		Speed:    speed,
	})
}

// SetError marks the download as failed
func (r *Runner) SetError(err error) {
	if r.program == nil {
		return
	}

	r.program.Send(ErrorMsg{Err: err})
}

// SetVerifying marks that checksum verification is in progress
func (r *Runner) SetVerifying() {
	if r.program == nil {
		return
	}

	r.program.Send(VerifyingMsg{})
}

// SetVerified marks checksum verification result
func (r *Runner) SetVerified(valid bool) {
	if r.program == nil {
		return
	}

	r.program.Send(VerifiedMsg{Valid: valid})
}

// Stop stops the TUI
func (r *Runner) Stop() {
	if r.program != nil {
		r.program.Quit()
	}
	r.cancel()
}

// Context returns the runner's context
func (r *Runner) Context() context.Context {
	return r.ctx
}

// IsPaused returns whether download is paused
func (r *Runner) IsPaused() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paused
}

// WaitIfPaused blocks if the download is paused
func (r *Runner) WaitIfPaused() {
	r.mu.Lock()
	if !r.paused {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	// Wait for resume signal
	select {
	case <-r.resumeCh:
	case <-r.ctx.Done():
	}
}

// Callbacks

func (r *Runner) onPause() {
	r.mu.Lock()
	r.paused = true
	r.mu.Unlock()
}

func (r *Runner) onResume() {
	r.mu.Lock()
	r.paused = false
	r.mu.Unlock()

	select {
	case r.resumeCh <- struct{}{}:
	default:
	}
}

func (r *Runner) onCancel() {
	r.cancel()
}

// ProgressCallback returns a callback function for the downloader
func (r *Runner) ProgressCallback() func(engine.Progress) {
	return func(p engine.Progress) {
		// Convert to chunk infos if available
		r.UpdateProgress(p, nil)
	}
}
