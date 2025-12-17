package ui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/download"
	"github.com/kilimcininkoroglu/burkut/internal/engine"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := FormatBytes(tt.bytes)
			if got != tt.expected {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestProgressBar_renderBar(t *testing.T) {
	p := NewProgressBar(WithNoColor(true))

	tests := []struct {
		percent float64
		width   int
	}{
		{0, 10},
		{50, 10},
		{100, 10},
		{25, 20},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			bar := p.renderBar(tt.percent, tt.width)
			// Should contain the percentage
			if !strings.Contains(bar, "%") {
				t.Errorf("Bar should contain percentage")
			}
		})
	}
}

func TestProgressBar_Render(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressBar(WithNoColor(true))

	progress := engine.Progress{
		Downloaded:   512 * 1024,
		TotalSize:    1024 * 1024,
		Speed:        100 * 1024,
		Percent:      50.0,
		ElapsedTime:  10 * time.Second,
		RemainingETA: 10 * time.Second,
		ChunkStatus: []engine.ChunkProgress{
			{ID: 0, Downloaded: 256 * 1024, Total: 512 * 1024, Status: download.ChunkStatusCompleted},
			{ID: 1, Downloaded: 256 * 1024, Total: 512 * 1024, Status: download.ChunkStatusInProgress},
		},
	}

	p.Render(&buf, progress, "test.zip")

	output := buf.String()

	// Should contain filename
	if !strings.Contains(output, "test.zip") {
		t.Error("Output should contain filename")
	}

	// Should contain progress percentage
	if !strings.Contains(output, "50.0%") {
		t.Error("Output should contain percentage")
	}

	// Should contain speed
	if !strings.Contains(output, "KB") {
		t.Error("Output should contain speed in KB")
	}
}

func TestProgressBar_RenderWithChunks(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressBar(WithNoColor(true), WithChunks(true))

	progress := engine.Progress{
		Downloaded:   512 * 1024,
		TotalSize:    1024 * 1024,
		Speed:        100 * 1024,
		Percent:      50.0,
		ElapsedTime:  10 * time.Second,
		RemainingETA: 10 * time.Second,
		ChunkStatus: []engine.ChunkProgress{
			{ID: 0, Downloaded: 512 * 1024, Total: 512 * 1024, Status: download.ChunkStatusCompleted},
			{ID: 1, Downloaded: 0, Total: 512 * 1024, Status: download.ChunkStatusInProgress},
		},
	}

	p.Render(&buf, progress, "test.zip")

	output := buf.String()

	// Should contain chunk info
	if !strings.Contains(output, "Chunk 0") {
		t.Error("Output should contain Chunk 0")
	}
	if !strings.Contains(output, "Chunk 1") {
		t.Error("Output should contain Chunk 1")
	}
}

func TestProgressBar_RenderComplete(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressBar(WithNoColor(true))

	progress := engine.Progress{
		Downloaded:   1024 * 1024,
		TotalSize:    1024 * 1024,
		Speed:        200 * 1024,
		Percent:      100.0,
		ElapsedTime:  5 * time.Second,
		RemainingETA: 0,
	}

	p.RenderComplete(&buf, progress, "test.zip")

	output := buf.String()

	// Should contain completion indicator
	if !strings.Contains(output, "completed") {
		t.Error("Output should contain 'completed'")
	}

	// Should contain filename
	if !strings.Contains(output, "test.zip") {
		t.Error("Output should contain filename")
	}
}

func TestProgressBar_RenderError(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressBar(WithNoColor(true))

	p.RenderError(&buf, "test.zip", fmt.Errorf("connection timeout"))

	output := buf.String()

	// Should contain error indicator
	if !strings.Contains(output, "failed") {
		t.Error("Output should contain 'failed'")
	}

	// Should contain error message
	if !strings.Contains(output, "connection timeout") {
		t.Error("Output should contain error message")
	}
}

func TestMinimalProgress(t *testing.T) {
	var buf bytes.Buffer

	progress := engine.Progress{
		Downloaded:   512 * 1024,
		TotalSize:    1024 * 1024,
		Speed:        100 * 1024,
		Percent:      50.0,
		RemainingETA: 5 * time.Second,
	}

	MinimalProgress(&buf, progress, "test.zip")

	output := buf.String()

	// Should be single line
	if strings.Count(output, "\n") > 0 {
		t.Error("Minimal progress should be single line without newline")
	}

	// Should contain filename
	if !strings.Contains(output, "test.zip") {
		t.Error("Output should contain filename")
	}

	// Should contain percentage
	if !strings.Contains(output, "50.0%") {
		t.Error("Output should contain percentage")
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer

	progress := engine.Progress{
		Downloaded:   512 * 1024,
		TotalSize:    1024 * 1024,
		Speed:        100 * 1024,
		Percent:      50.0,
		RemainingETA: 5 * time.Second,
	}

	RenderJSON(&buf, progress, "test.zip")

	output := buf.String()

	// Should be valid JSON-like structure
	if !strings.HasPrefix(output, "{") {
		t.Error("JSON output should start with {")
	}

	if !strings.HasSuffix(strings.TrimSpace(output), "}") {
		t.Error("JSON output should end with }")
	}

	// Should contain expected fields
	expectedFields := []string{
		`"filename":"test.zip"`,
		`"percent":50.0`,
		`"downloaded":524288`,
		`"total":1048576`,
	}

	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Errorf("JSON output should contain %s", field)
		}
	}
}

func TestProgressBar_formatDuration(t *testing.T) {
	p := NewProgressBar()

	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "00:00"},
		{30 * time.Second, "00:30"},
		{90 * time.Second, "01:30"},
		{3600 * time.Second, "01:00:00"},
		{3661 * time.Second, "01:01:01"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := p.formatDuration(tt.duration)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestProgressBar_formatSpeed(t *testing.T) {
	p := NewProgressBar(WithNoColor(true))

	tests := []struct {
		speed    int64
		contains string
	}{
		{0, "-- B/s"},
		{1024, "KB/s"},
		{1048576, "MB/s"},
	}

	for _, tt := range tests {
		t.Run(tt.contains, func(t *testing.T) {
			got := p.formatSpeed(tt.speed)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("formatSpeed(%d) = %q, should contain %q", tt.speed, got, tt.contains)
			}
		})
	}
}


