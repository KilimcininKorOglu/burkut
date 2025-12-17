package download

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChunk_Size(t *testing.T) {
	chunk := Chunk{Start: 0, End: 99}
	if chunk.Size() != 100 {
		t.Errorf("Size() = %d, want 100", chunk.Size())
	}

	chunk2 := Chunk{Start: 100, End: 199}
	if chunk2.Size() != 100 {
		t.Errorf("Size() = %d, want 100", chunk2.Size())
	}
}

func TestChunk_Remaining(t *testing.T) {
	chunk := Chunk{Start: 0, End: 99, Downloaded: 50}
	if chunk.Remaining() != 50 {
		t.Errorf("Remaining() = %d, want 50", chunk.Remaining())
	}
}

func TestChunk_IsComplete(t *testing.T) {
	tests := []struct {
		name       string
		downloaded int64
		end        int64
		want       bool
	}{
		{"not complete", 50, 99, false},
		{"exactly complete", 100, 99, true},
		{"over complete", 150, 99, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk := Chunk{Start: 0, End: tt.end, Downloaded: tt.downloaded}
			if got := chunk.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChunk_CurrentPosition(t *testing.T) {
	chunk := Chunk{Start: 1000, End: 1999, Downloaded: 500}
	if chunk.CurrentPosition() != 1500 {
		t.Errorf("CurrentPosition() = %d, want 1500", chunk.CurrentPosition())
	}
}

func TestNewState(t *testing.T) {
	state := NewState("http://example.com/file.zip", "file.zip", 1024, true)

	if state.URL != "http://example.com/file.zip" {
		t.Errorf("URL = %q, want %q", state.URL, "http://example.com/file.zip")
	}

	if state.Filename != "file.zip" {
		t.Errorf("Filename = %q, want %q", state.Filename, "file.zip")
	}

	if state.TotalSize != 1024 {
		t.Errorf("TotalSize = %d, want 1024", state.TotalSize)
	}

	if !state.AcceptRange {
		t.Error("AcceptRange should be true")
	}

	if state.Version != StateVersion {
		t.Errorf("Version = %d, want %d", state.Version, StateVersion)
	}
}

func TestState_InitializeChunks(t *testing.T) {
	tests := []struct {
		name       string
		totalSize  int64
		numChunks  int
		wantChunks int
	}{
		{"4 chunks for 1000 bytes", 1000, 4, 4},
		{"1 chunk for 100 bytes", 100, 1, 1},
		{"8 chunks for 1024 bytes", 1024, 8, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewState("http://example.com/file.zip", "file.zip", tt.totalSize, true)
			state.InitializeChunks(tt.numChunks)

			if len(state.Chunks) != tt.wantChunks {
				t.Errorf("len(Chunks) = %d, want %d", len(state.Chunks), tt.wantChunks)
			}

			// Verify chunks cover entire file
			var totalCovered int64
			for _, chunk := range state.Chunks {
				totalCovered += chunk.Size()
			}

			if totalCovered != tt.totalSize {
				t.Errorf("Total covered = %d, want %d", totalCovered, tt.totalSize)
			}

			// Verify chunks are contiguous
			for i := 1; i < len(state.Chunks); i++ {
				if state.Chunks[i].Start != state.Chunks[i-1].End+1 {
					t.Errorf("Chunk %d start = %d, expected %d",
						i, state.Chunks[i].Start, state.Chunks[i-1].End+1)
				}
			}
		})
	}
}

func TestState_InitializeChunks_UnknownSize(t *testing.T) {
	state := NewState("http://example.com/file.zip", "file.zip", 0, false)
	state.InitializeChunks(4)

	// Should create single chunk with unknown end
	if len(state.Chunks) != 1 {
		t.Errorf("len(Chunks) = %d, want 1", len(state.Chunks))
	}

	if state.Chunks[0].End != -1 {
		t.Errorf("Chunk end = %d, want -1 for unknown size", state.Chunks[0].End)
	}
}

func TestState_UpdateChunk(t *testing.T) {
	state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
	state.InitializeChunks(4)

	// Update first chunk
	state.UpdateChunk(0, 200, ChunkStatusInProgress)

	chunk, ok := state.GetChunk(0)
	if !ok {
		t.Fatal("GetChunk(0) returned false")
	}

	if chunk.Downloaded != 200 {
		t.Errorf("Downloaded = %d, want 200", chunk.Downloaded)
	}

	if chunk.Status != ChunkStatusInProgress {
		t.Errorf("Status = %q, want %q", chunk.Status, ChunkStatusInProgress)
	}

	// Total downloaded should be updated
	if state.Downloaded != 200 {
		t.Errorf("State.Downloaded = %d, want 200", state.Downloaded)
	}
}

func TestState_GetPendingChunks(t *testing.T) {
	state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
	state.InitializeChunks(4)

	// Complete first two chunks
	state.UpdateChunk(0, 250, ChunkStatusCompleted)
	state.UpdateChunk(1, 250, ChunkStatusCompleted)

	pending := state.GetPendingChunks()
	if len(pending) != 2 {
		t.Errorf("len(pending) = %d, want 2", len(pending))
	}

	// Should be chunks 2 and 3
	ids := make(map[int]bool)
	for _, chunk := range pending {
		ids[chunk.ID] = true
	}

	if !ids[2] || !ids[3] {
		t.Error("Pending chunks should be 2 and 3")
	}
}

func TestState_Progress(t *testing.T) {
	state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
	state.InitializeChunks(4)

	// No progress
	if state.Progress() != 0 {
		t.Errorf("Progress() = %f, want 0", state.Progress())
	}

	// 50% progress
	state.UpdateChunk(0, 250, ChunkStatusCompleted)
	state.UpdateChunk(1, 250, ChunkStatusCompleted)

	progress := state.Progress()
	if progress != 50 {
		t.Errorf("Progress() = %f, want 50", progress)
	}

	// 100% progress
	state.UpdateChunk(2, 250, ChunkStatusCompleted)
	state.UpdateChunk(3, 250, ChunkStatusCompleted)

	progress = state.Progress()
	if progress != 100 {
		t.Errorf("Progress() = %f, want 100", progress)
	}
}

func TestState_IsComplete(t *testing.T) {
	state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
	state.InitializeChunks(2)

	if state.IsComplete() {
		t.Error("IsComplete() should be false initially")
	}

	state.UpdateChunk(0, 500, ChunkStatusCompleted)
	if state.IsComplete() {
		t.Error("IsComplete() should be false with one chunk remaining")
	}

	state.UpdateChunk(1, 500, ChunkStatusCompleted)
	if !state.IsComplete() {
		t.Error("IsComplete() should be true when all chunks completed")
	}
}

func TestState_SaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	downloadPath := filepath.Join(tmpDir, "file.zip")

	// Create and save state
	state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
	state.InitializeChunks(4)
	state.UpdateChunk(0, 250, ChunkStatusCompleted)
	state.UpdateChunk(1, 100, ChunkStatusInProgress)
	state.Checksum = &Checksum{Algorithm: "sha256", Expected: "abc123"}

	if err := state.Save(downloadPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load state
	loaded, err := LoadState(downloadPath)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	// Verify loaded state
	if loaded.URL != state.URL {
		t.Errorf("URL = %q, want %q", loaded.URL, state.URL)
	}

	if loaded.TotalSize != state.TotalSize {
		t.Errorf("TotalSize = %d, want %d", loaded.TotalSize, state.TotalSize)
	}

	if len(loaded.Chunks) != len(state.Chunks) {
		t.Errorf("len(Chunks) = %d, want %d", len(loaded.Chunks), len(state.Chunks))
	}

	if loaded.Downloaded != state.Downloaded {
		t.Errorf("Downloaded = %d, want %d", loaded.Downloaded, state.Downloaded)
	}

	if loaded.Checksum == nil || loaded.Checksum.Algorithm != "sha256" {
		t.Error("Checksum not loaded correctly")
	}
}

func TestState_SaveLoad_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	downloadPath := filepath.Join(tmpDir, "file.zip")

	state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
	state.InitializeChunks(1)

	// Save multiple times to test atomic write
	for i := 0; i < 10; i++ {
		state.UpdateChunk(0, int64(i*100), ChunkStatusInProgress)
		if err := state.Save(downloadPath); err != nil {
			t.Fatalf("Save() iteration %d error = %v", i, err)
		}
	}

	// Temp file should not exist after save
	tmpPath := StateFilePath(downloadPath) + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("Temporary file should not exist after save")
	}
}

func TestStateExists(t *testing.T) {
	tmpDir := t.TempDir()
	downloadPath := filepath.Join(tmpDir, "file.zip")

	if StateExists(downloadPath) {
		t.Error("StateExists() should be false when no state file")
	}

	state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
	state.Save(downloadPath)

	if !StateExists(downloadPath) {
		t.Error("StateExists() should be true after save")
	}
}

func TestDeleteState(t *testing.T) {
	tmpDir := t.TempDir()
	downloadPath := filepath.Join(tmpDir, "file.zip")

	state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
	state.Save(downloadPath)

	if err := DeleteState(downloadPath); err != nil {
		t.Fatalf("DeleteState() error = %v", err)
	}

	if StateExists(downloadPath) {
		t.Error("State file should not exist after delete")
	}

	// Delete non-existent should not error
	if err := DeleteState(downloadPath); err != nil {
		t.Errorf("DeleteState() on non-existent should not error: %v", err)
	}
}

func TestStateStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	// Create some state files
	for i := 0; i < 3; i++ {
		state := NewState("http://example.com/file.zip", "file.zip", 1000, true)
		state.Save(filepath.Join(tmpDir, "file"+string(rune('A'+i))+".zip"))
	}

	states, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(states) != 3 {
		t.Errorf("len(states) = %d, want 3", len(states))
	}
}

func TestStateStore_Clean(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	// Create completed and incomplete states
	completePath := filepath.Join(tmpDir, "complete.zip")
	completeState := NewState("http://example.com/complete.zip", "complete.zip", 100, true)
	completeState.InitializeChunks(1)
	completeState.UpdateChunk(0, 100, ChunkStatusCompleted)
	completeState.Save(completePath)

	incompletePath := filepath.Join(tmpDir, "incomplete.zip")
	incompleteState := NewState("http://example.com/incomplete.zip", "incomplete.zip", 100, true)
	incompleteState.InitializeChunks(1)
	incompleteState.UpdateChunk(0, 50, ChunkStatusInProgress)
	incompleteState.Save(incompletePath)

	if err := store.Clean(); err != nil {
		t.Fatalf("Clean() error = %v", err)
	}

	// Complete state should be deleted
	if StateExists(completePath) {
		t.Error("Completed state file should be deleted")
	}

	// Incomplete state should remain
	if !StateExists(incompletePath) {
		t.Error("Incomplete state file should remain")
	}
}

func TestStateFilePath(t *testing.T) {
	path := StateFilePath("/downloads/file.zip")
	expected := "/downloads/file.zip.burkut-state"
	if path != expected {
		t.Errorf("StateFilePath() = %q, want %q", path, expected)
	}
}

// Ensure time is used
var _ = time.Now
