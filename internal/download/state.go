// Package download provides download management and state persistence.
package download

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ChunkStatus represents the status of a download chunk
type ChunkStatus string

const (
	ChunkStatusPending    ChunkStatus = "pending"
	ChunkStatusInProgress ChunkStatus = "in_progress"
	ChunkStatusCompleted  ChunkStatus = "completed"
	ChunkStatusFailed     ChunkStatus = "failed"
)

// Chunk represents a portion of a file being downloaded
type Chunk struct {
	ID         int         `json:"id"`
	Start      int64       `json:"start"`
	End        int64       `json:"end"`
	Downloaded int64       `json:"downloaded"`
	Status     ChunkStatus `json:"status"`
}

// Size returns the total size of this chunk
func (c *Chunk) Size() int64 {
	return c.End - c.Start + 1
}

// Remaining returns the number of bytes left to download
func (c *Chunk) Remaining() int64 {
	return c.Size() - c.Downloaded
}

// IsComplete returns true if the chunk is fully downloaded
func (c *Chunk) IsComplete() bool {
	return c.Downloaded >= c.Size()
}

// CurrentPosition returns the current byte position for resuming
func (c *Chunk) CurrentPosition() int64 {
	return c.Start + c.Downloaded
}

// Checksum represents checksum information for verification
type Checksum struct {
	Algorithm string `json:"algorithm"`
	Expected  string `json:"expected"`
	Actual    string `json:"actual,omitempty"`
}

// State represents the complete state of a download
type State struct {
	Version     int       `json:"version"`
	URL         string    `json:"url"`
	Filename    string    `json:"filename"`
	TotalSize   int64     `json:"total_size"`
	Downloaded  int64     `json:"downloaded"`
	Chunks      []Chunk   `json:"chunks"`
	Checksum    *Checksum `json:"checksum,omitempty"`
	AcceptRange bool      `json:"accept_range"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	mu sync.RWMutex `json:"-"`
}

const (
	// StateVersion is the current state file format version
	StateVersion = 1

	// StateFileSuffix is the suffix for state files
	StateFileSuffix = ".burkut-state"
)

// NewState creates a new State for a download
func NewState(url, filename string, totalSize int64, acceptRange bool) *State {
	now := time.Now()
	return &State{
		Version:     StateVersion,
		URL:         url,
		Filename:    filename,
		TotalSize:   totalSize,
		AcceptRange: acceptRange,
		Chunks:      make([]Chunk, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// InitializeChunks creates chunks for parallel download
func (s *State) InitializeChunks(numChunks int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.TotalSize <= 0 || numChunks <= 0 {
		// Unknown size or invalid chunks - single chunk mode
		s.Chunks = []Chunk{{
			ID:     0,
			Start:  0,
			End:    -1, // Unknown end
			Status: ChunkStatusPending,
		}}
		return
	}

	chunkSize := s.TotalSize / int64(numChunks)
	s.Chunks = make([]Chunk, numChunks)

	for i := 0; i < numChunks; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1

		// Last chunk gets any remaining bytes
		if i == numChunks-1 {
			end = s.TotalSize - 1
		}

		s.Chunks[i] = Chunk{
			ID:     i,
			Start:  start,
			End:    end,
			Status: ChunkStatusPending,
		}
	}
}

// UpdateChunk updates a chunk's progress
func (s *State) UpdateChunk(chunkID int, downloaded int64, status ChunkStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if chunkID < 0 || chunkID >= len(s.Chunks) {
		return
	}

	s.Chunks[chunkID].Downloaded = downloaded
	s.Chunks[chunkID].Status = status
	s.UpdatedAt = time.Now()

	// Recalculate total downloaded
	s.recalculateDownloaded()
}

// recalculateDownloaded recalculates total downloaded bytes from chunks
// Must be called with lock held
func (s *State) recalculateDownloaded() {
	var total int64
	for _, chunk := range s.Chunks {
		total += chunk.Downloaded
	}
	s.Downloaded = total
}

// GetChunk returns a chunk by ID
func (s *State) GetChunk(chunkID int) (*Chunk, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if chunkID < 0 || chunkID >= len(s.Chunks) {
		return nil, false
	}

	chunk := s.Chunks[chunkID]
	return &chunk, true
}

// GetPendingChunks returns all chunks that are not completed
func (s *State) GetPendingChunks() []Chunk {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pending := make([]Chunk, 0)
	for _, chunk := range s.Chunks {
		if chunk.Status != ChunkStatusCompleted {
			pending = append(pending, chunk)
		}
	}
	return pending
}

// Progress returns the download progress as a percentage (0-100)
func (s *State) Progress() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.TotalSize <= 0 {
		return 0
	}
	return float64(s.Downloaded) / float64(s.TotalSize) * 100
}

// IsComplete returns true if all chunks are downloaded
func (s *State) IsComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Chunks) == 0 {
		return false
	}

	for _, chunk := range s.Chunks {
		if chunk.Status != ChunkStatusCompleted {
			return false
		}
	}
	return true
}

// StateFilePath returns the path to the state file for a given download file
func StateFilePath(downloadPath string) string {
	return downloadPath + StateFileSuffix
}

// Save writes the state to a JSON file
func (s *State) Save(downloadPath string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statePath := StateFilePath(downloadPath)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	// Write to temp file first, then rename for atomicity
	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}

	if err := os.Rename(tmpPath, statePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("renaming state file: %w", err)
	}

	return nil
}

// LoadState loads a state from a JSON file
func LoadState(downloadPath string) (*State, error) {
	statePath := StateFilePath(downloadPath)

	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshaling state: %w", err)
	}

	// Version check for future compatibility
	if state.Version > StateVersion {
		return nil, fmt.Errorf("state file version %d is newer than supported version %d",
			state.Version, StateVersion)
	}

	return &state, nil
}

// StateExists checks if a state file exists for the given download path
func StateExists(downloadPath string) bool {
	statePath := StateFilePath(downloadPath)
	_, err := os.Stat(statePath)
	return err == nil
}

// DeleteState removes the state file for the given download path
func DeleteState(downloadPath string) error {
	statePath := StateFilePath(downloadPath)
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing state file: %w", err)
	}
	return nil
}

// StateStore manages multiple download states
type StateStore struct {
	baseDir string
}

// NewStateStore creates a new StateStore with the given base directory
func NewStateStore(baseDir string) *StateStore {
	return &StateStore{baseDir: baseDir}
}

// List returns all state files in the store directory
func (ss *StateStore) List() ([]*State, error) {
	pattern := filepath.Join(ss.baseDir, "*"+StateFileSuffix)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("globbing state files: %w", err)
	}

	states := make([]*State, 0, len(matches))
	for _, match := range matches {
		// Extract download path from state path
		downloadPath := match[:len(match)-len(StateFileSuffix)]
		state, err := LoadState(downloadPath)
		if err != nil {
			// Skip corrupted state files
			continue
		}
		states = append(states, state)
	}

	return states, nil
}

// Clean removes state files for completed downloads
func (ss *StateStore) Clean() error {
	states, err := ss.List()
	if err != nil {
		return err
	}

	for _, state := range states {
		if state.IsComplete() {
			downloadPath := filepath.Join(ss.baseDir, state.Filename)
			DeleteState(downloadPath)
		}
	}

	return nil
}
