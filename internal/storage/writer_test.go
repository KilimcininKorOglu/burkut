package storage

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileWriter(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")

	// Test creating a new file
	w, err := NewFileWriter(path, 1024)
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}
	defer w.Close()

	if w.Path() != path {
		t.Errorf("Path() = %q, want %q", w.Path(), path)
	}

	if w.Size() != 1024 {
		t.Errorf("Size() = %d, want 1024", w.Size())
	}

	// Verify file was created
	if !FileExists(path) {
		t.Error("File was not created")
	}

	// Verify file size (pre-allocated)
	size, err := FileSize(path)
	if err != nil {
		t.Fatalf("FileSize() error = %v", err)
	}
	if size != 1024 {
		t.Errorf("FileSize() = %d, want 1024", size)
	}
}

func TestNewFileWriter_WithDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "nested", "test.bin")

	w, err := NewFileWriter(path, 100)
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}
	defer w.Close()

	if !FileExists(path) {
		t.Error("File was not created in nested directory")
	}
}

func TestFileWriter_Write(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")

	w, err := NewFileWriter(path, 0)
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}

	// Write some data
	data := []byte("Hello, Burkut!")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() = %d bytes, want %d", n, len(data))
	}

	// Check written count
	if w.Written() != int64(len(data)) {
		t.Errorf("Written() = %d, want %d", w.Written(), len(data))
	}

	// Close and verify content
	w.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != string(data) {
		t.Errorf("File content = %q, want %q", string(content), string(data))
	}
}

func TestFileWriter_WriteAt(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")

	// Create file with pre-allocated size
	w, err := NewFileWriter(path, 20)
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}

	// Write at different offsets (simulating parallel chunks)
	chunks := []struct {
		data   []byte
		offset int64
	}{
		{[]byte("AAAAA"), 0},
		{[]byte("BBBBB"), 5},
		{[]byte("CCCCC"), 10},
		{[]byte("DDDDD"), 15},
	}

	for _, chunk := range chunks {
		n, err := w.WriteAt(chunk.data, chunk.offset)
		if err != nil {
			t.Fatalf("WriteAt(%d) error = %v", chunk.offset, err)
		}
		if n != len(chunk.data) {
			t.Errorf("WriteAt(%d) = %d bytes, want %d", chunk.offset, n, len(chunk.data))
		}
	}

	w.Close()

	// Verify content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	expected := "AAAAABBBBBCCCCCDDDD"
	if string(content) != expected+"D" { // Last byte from preallocate
		t.Errorf("File content = %q, want %q", string(content), expected+"D")
	}
}

func TestFileWriter_WriteChunk(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")

	w, err := NewFileWriter(path, 30)
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}

	// Write chunks using readers
	chunks := []struct {
		data   string
		offset int64
		size   int64
	}{
		{"0123456789", 0, 10},
		{"ABCDEFGHIJ", 10, 10},
		{"abcdefghij", 20, 10},
	}

	for _, chunk := range chunks {
		reader := bytes.NewReader([]byte(chunk.data))
		written, err := w.WriteChunk(reader, chunk.offset, chunk.size)
		if err != nil {
			t.Fatalf("WriteChunk(%d) error = %v", chunk.offset, err)
		}
		if written != chunk.size {
			t.Errorf("WriteChunk(%d) = %d bytes, want %d", chunk.offset, written, chunk.size)
		}
	}

	w.Close()

	// Verify content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	expected := "0123456789ABCDEFGHIJabcdefghij"
	if string(content) != expected {
		t.Errorf("File content = %q, want %q", string(content), expected)
	}
}

func TestFileWriter_Sync(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")

	w, err := NewFileWriter(path, 0)
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}
	defer w.Close()

	w.Write([]byte("test data"))

	// Sync should not return error
	if err := w.Sync(); err != nil {
		t.Errorf("Sync() error = %v", err)
	}
}

func TestFileWriter_Truncate(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")

	w, err := NewFileWriter(path, 100)
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}

	// Truncate to smaller size
	if err := w.Truncate(50); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}

	w.Close()

	// Verify new size
	size, err := FileSize(path)
	if err != nil {
		t.Fatalf("FileSize() error = %v", err)
	}
	if size != 50 {
		t.Errorf("FileSize() after truncate = %d, want 50", size)
	}
}

func TestFileWriter_ClosedOperations(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")

	w, err := NewFileWriter(path, 0)
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}

	w.Close()

	// Operations on closed writer should fail
	if _, err := w.Write([]byte("test")); err == nil {
		t.Error("Write() on closed writer should fail")
	}

	if _, err := w.WriteAt([]byte("test"), 0); err == nil {
		t.Error("WriteAt() on closed writer should fail")
	}

	if err := w.Sync(); err == nil {
		t.Error("Sync() on closed writer should fail")
	}

	if err := w.Truncate(0); err == nil {
		t.Error("Truncate() on closed writer should fail")
	}

	// Double close should not panic
	if err := w.Close(); err != nil {
		t.Errorf("Double Close() error = %v", err)
	}
}

func TestOpenFileWriter(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")

	// Create file with initial content
	if err := os.WriteFile(path, []byte("initial content"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Open for resume
	w, err := OpenFileWriter(path, 100)
	if err != nil {
		t.Fatalf("OpenFileWriter() error = %v", err)
	}
	defer w.Close()

	// Written should reflect existing content
	if w.Written() != 15 { // len("initial content")
		t.Errorf("Written() = %d, want 15", w.Written())
	}

	// Can write more data
	w.WriteAt([]byte("more"), 15)
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	existingPath := filepath.Join(tmpDir, "exists.txt")
	nonExistingPath := filepath.Join(tmpDir, "not-exists.txt")

	// Create a file
	if err := os.WriteFile(existingPath, []byte("test"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if !FileExists(existingPath) {
		t.Error("FileExists() = false for existing file")
	}

	if FileExists(nonExistingPath) {
		t.Error("FileExists() = true for non-existing file")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	content := []byte("content to copy")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	copied, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !bytes.Equal(content, copied) {
		t.Errorf("Copied content = %q, want %q", string(copied), string(content))
	}
}

func TestRemoveFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "to-remove.txt")

	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := RemoveFile(path); err != nil {
		t.Fatalf("RemoveFile() error = %v", err)
	}

	if FileExists(path) {
		t.Error("File still exists after removal")
	}
}

// BenchmarkWriteAt benchmarks parallel write performance
func BenchmarkWriteAt(b *testing.B) {
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "bench.bin")

	w, err := NewFileWriter(path, int64(b.N*1024))
	if err != nil {
		b.Fatalf("NewFileWriter() error = %v", err)
	}
	defer w.Close()

	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.WriteAt(data, int64(i*1024))
	}
}

// BenchmarkWriteChunk benchmarks chunk write with io.Copy
func BenchmarkWriteChunk(b *testing.B) {
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "bench.bin")

	w, err := NewFileWriter(path, int64(b.N*1024))
	if err != nil {
		b.Fatalf("NewFileWriter() error = %v", err)
	}
	defer w.Close()

	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		w.WriteChunk(reader, int64(i*1024), 1024)
	}
}

// Ensure io import is used
var _ io.Reader = (*bytes.Reader)(nil)
