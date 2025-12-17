// Package storage provides file I/O operations for downloads.
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// FileWriter handles writing downloaded data to disk.
// It supports both sequential and random-access writes for chunk-based downloads.
type FileWriter struct {
	file     *os.File
	path     string
	size     int64
	written  int64
	mu       sync.Mutex
	closed   bool
}

// NewFileWriter creates a new FileWriter for the given path.
// If size > 0, it pre-allocates the file to that size (sparse file on supported systems).
func NewFileWriter(path string, size int64) (*FileWriter, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Open file for writing (create if not exists, truncate if exists)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening file %s: %w", path, err)
	}

	fw := &FileWriter{
		file: file,
		path: path,
		size: size,
	}

	// Pre-allocate file if size is known
	if size > 0 {
		if err := fw.preallocate(size); err != nil {
			file.Close()
			return nil, fmt.Errorf("preallocating file: %w", err)
		}
	}

	return fw, nil
}

// OpenFileWriter opens an existing file for resuming a download.
// It doesn't truncate the file.
func OpenFileWriter(path string, size int64) (*FileWriter, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening file %s: %w", path, err)
	}

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("getting file info: %w", err)
	}

	return &FileWriter{
		file:    file,
		path:    path,
		size:    size,
		written: info.Size(),
	}, nil
}

// preallocate sets the file size without writing zeros (sparse file)
func (w *FileWriter) preallocate(size int64) error {
	// Seek to size-1 and write a single byte to create sparse file
	// This is more efficient than truncate on some filesystems
	if _, err := w.file.Seek(size-1, io.SeekStart); err != nil {
		return err
	}
	if _, err := w.file.Write([]byte{0}); err != nil {
		return err
	}
	// Seek back to beginning
	_, err := w.file.Seek(0, io.SeekStart)
	return err
}

// Write writes data sequentially to the file.
// This is the io.Writer interface implementation.
func (w *FileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, fmt.Errorf("writer is closed")
	}

	n, err := w.file.Write(p)
	w.written += int64(n)
	return n, err
}

// WriteAt writes data at a specific offset.
// This is used for parallel chunk downloads.
func (w *FileWriter) WriteAt(p []byte, offset int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, fmt.Errorf("writer is closed")
	}

	n, err := w.file.WriteAt(p, offset)
	return n, err
}

// WriteChunk writes data from a reader at a specific offset.
// It returns the number of bytes written.
func (w *FileWriter) WriteChunk(r io.Reader, offset int64, size int64) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, fmt.Errorf("writer is closed")
	}

	// Seek to offset
	if _, err := w.file.Seek(offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seeking to offset %d: %w", offset, err)
	}

	// Copy data with a limit if size is specified
	var written int64
	var err error

	if size > 0 {
		written, err = io.CopyN(w.file, r, size)
	} else {
		written, err = io.Copy(w.file, r)
	}

	return written, err
}

// Sync flushes the file to disk.
func (w *FileWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	return w.file.Sync()
}

// Close closes the file.
func (w *FileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	return w.file.Close()
}

// Path returns the file path.
func (w *FileWriter) Path() string {
	return w.path
}

// Size returns the expected total size.
func (w *FileWriter) Size() int64 {
	return w.size
}

// Written returns the number of bytes written (for sequential writes).
func (w *FileWriter) Written() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.written
}

// Truncate truncates the file to the specified size.
func (w *FileWriter) Truncate(size int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	return w.file.Truncate(size)
}

// FileExists checks if a file exists at the given path.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FileSize returns the size of the file at the given path.
func FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// RemoveFile removes the file at the given path.
func RemoveFile(path string) error {
	return os.Remove(path)
}

// TempFile creates a temporary file with the given prefix.
func TempFile(dir, prefix string) (*os.File, error) {
	return os.CreateTemp(dir, prefix)
}

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return fmt.Errorf("copying file: %w", err)
	}

	return destination.Sync()
}
