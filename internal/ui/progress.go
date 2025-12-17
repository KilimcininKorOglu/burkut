// Package ui provides terminal user interface components.
package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/download"
	"github.com/kilimcininkoroglu/burkut/internal/engine"
)

// ProgressBar displays download progress in the terminal
type ProgressBar struct {
	output    io.Writer
	width     int
	showChunks bool
	noColor   bool
	lastLines int
}

// ProgressBarOption configures a ProgressBar
type ProgressBarOption func(*ProgressBar)

// WithOutput sets the output writer
func WithOutput(w io.Writer) ProgressBarOption {
	return func(p *ProgressBar) {
		p.output = w
	}
}

// WithWidth sets the progress bar width
func WithWidth(width int) ProgressBarOption {
	return func(p *ProgressBar) {
		p.width = width
	}
}

// WithChunks enables chunk progress display
func WithChunks(show bool) ProgressBarOption {
	return func(p *ProgressBar) {
		p.showChunks = show
	}
}

// WithNoColor disables colored output
func WithNoColor(noColor bool) ProgressBarOption {
	return func(p *ProgressBar) {
		p.noColor = noColor
	}
}

// NewProgressBar creates a new ProgressBar
func NewProgressBar(opts ...ProgressBarOption) *ProgressBar {
	p := &ProgressBar{
		output:    nil, // Will use os.Stdout by default in Render
		width:     40,
		showChunks: false,
		noColor:   false,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	clearLine   = "\033[2K"
	moveUp      = "\033[%dA"
)

// Render renders the progress bar to the output
func (p *ProgressBar) Render(w io.Writer, progress engine.Progress, filename string) {
	var sb strings.Builder

	// Clear previous output
	if p.lastLines > 0 {
		// Move cursor up and clear lines
		for i := 0; i < p.lastLines; i++ {
			sb.WriteString(clearLine + "\r")
			if i < p.lastLines-1 {
				sb.WriteString(fmt.Sprintf(moveUp, 1))
			}
		}
		sb.WriteString(fmt.Sprintf(moveUp, p.lastLines-1))
	}

	lines := 0

	// Filename
	sb.WriteString(p.color(colorBold, filename) + "\n")
	lines++

	// Main progress bar
	bar := p.renderBar(progress.Percent, p.width)
	sizeStr := p.formatSize(progress.Downloaded, progress.TotalSize)
	sb.WriteString(fmt.Sprintf("  %s %s\n", bar, sizeStr))
	lines++

	// Stats line
	speedStr := p.formatSpeed(progress.Speed)
	etaStr := p.formatETA(progress.RemainingETA)
	elapsedStr := p.formatDuration(progress.ElapsedTime)

	sb.WriteString(fmt.Sprintf("  Speed: %s  |  ETA: %s  |  Elapsed: %s\n",
		p.color(colorCyan, speedStr),
		p.color(colorYellow, etaStr),
		elapsedStr))
	lines++

	// Chunk progress (optional)
	if p.showChunks && len(progress.ChunkStatus) > 1 {
		sb.WriteString("\n")
		lines++
		for _, chunk := range progress.ChunkStatus {
			chunkPercent := float64(0)
			if chunk.Total > 0 {
				chunkPercent = float64(chunk.Downloaded) / float64(chunk.Total) * 100
			}
			chunkBar := p.renderMiniBar(chunkPercent, 20)
			statusIcon := p.chunkStatusIcon(chunk.Status)
			sb.WriteString(fmt.Sprintf("  [Chunk %d: %s %s]\n", chunk.ID, chunkBar, statusIcon))
			lines++
		}
	}

	p.lastLines = lines
	fmt.Fprint(w, sb.String())
}

// RenderComplete renders the completion message
func (p *ProgressBar) RenderComplete(w io.Writer, progress engine.Progress, filename string) {
	// Clear progress display
	if p.lastLines > 0 {
		for i := 0; i < p.lastLines; i++ {
			fmt.Fprintf(w, clearLine+"\r")
			if i < p.lastLines-1 {
				fmt.Fprintf(w, moveUp, 1)
			}
		}
		fmt.Fprintf(w, moveUp, p.lastLines-1)
	}

	// Print completion message
	checkmark := p.color(colorGreen, "✓")
	sizeStr := FormatBytes(progress.TotalSize)
	speedStr := p.formatSpeed(progress.Speed)
	timeStr := p.formatDuration(progress.ElapsedTime)

	fmt.Fprintf(w, "%s %s %s (%s, %s)\n",
		checkmark,
		p.color(colorBold, filename),
		p.color(colorGreen, "completed"),
		sizeStr,
		fmt.Sprintf("%s in %s", speedStr, timeStr))

	p.lastLines = 0
}

// RenderError renders an error message
func (p *ProgressBar) RenderError(w io.Writer, filename string, err error) {
	// Clear progress display
	if p.lastLines > 0 {
		for i := 0; i < p.lastLines; i++ {
			fmt.Fprintf(w, clearLine+"\r")
			if i < p.lastLines-1 {
				fmt.Fprintf(w, moveUp, 1)
			}
		}
		fmt.Fprintf(w, moveUp, p.lastLines-1)
	}

	cross := p.color(colorYellow, "✗")
	fmt.Fprintf(w, "%s %s %s: %v\n",
		cross,
		p.color(colorBold, filename),
		p.color(colorYellow, "failed"),
		err)

	p.lastLines = 0
}

// renderBar creates an ASCII progress bar
func (p *ProgressBar) renderBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int(float64(width) * percent / 100)
	empty := width - filled

	bar := strings.Repeat("━", filled) + strings.Repeat("─", empty)
	percentStr := fmt.Sprintf("%5.1f%%", percent)

	return p.color(colorGreen, bar) + " " + percentStr
}

// renderMiniBar creates a small progress bar for chunks
func (p *ProgressBar) renderMiniBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int(float64(width) * percent / 100)
	empty := width - filled

	return p.color(colorGreen, strings.Repeat("█", filled)) +
		strings.Repeat("░", empty)
}

// chunkStatusIcon returns an icon for chunk status
func (p *ProgressBar) chunkStatusIcon(status download.ChunkStatus) string {
	switch status {
	case download.ChunkStatusCompleted:
		return p.color(colorGreen, "✓")
	case download.ChunkStatusInProgress:
		return p.color(colorCyan, "↓")
	case download.ChunkStatusFailed:
		return p.color(colorYellow, "✗")
	default:
		return "○"
	}
}

// formatSize formats downloaded/total size
func (p *ProgressBar) formatSize(downloaded, total int64) string {
	if total <= 0 {
		return FormatBytes(downloaded)
	}
	return fmt.Sprintf("%s/%s", FormatBytes(downloaded), FormatBytes(total))
}

// formatSpeed formats download speed
func (p *ProgressBar) formatSpeed(bytesPerSec int64) string {
	if bytesPerSec <= 0 {
		return "-- B/s"
	}
	return FormatBytes(bytesPerSec) + "/s"
}

// formatETA formats estimated time remaining
func (p *ProgressBar) formatETA(eta time.Duration) string {
	if eta <= 0 {
		return "--:--"
	}
	return p.formatDuration(eta)
}

// formatDuration formats a duration as mm:ss or hh:mm:ss
func (p *ProgressBar) formatDuration(d time.Duration) string {
	if d <= 0 {
		return "00:00"
	}

	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// color wraps text in ANSI color codes
func (p *ProgressBar) color(code, text string) string {
	if p.noColor {
		return text
	}
	return code + text + colorReset
}

// FormatBytes formats bytes into human-readable string
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// MinimalProgress renders a single-line progress output
func MinimalProgress(w io.Writer, progress engine.Progress, filename string) {
	bar := ""
	width := 25
	if progress.TotalSize > 0 {
		filled := int(float64(width) * progress.Percent / 100)
		bar = "[" + strings.Repeat("=", filled) + ">" + strings.Repeat(" ", width-filled-1) + "]"
	}

	fmt.Fprintf(w, "\r%s: %.1f%% %s %s %s eta %s",
		filename,
		progress.Percent,
		bar,
		FormatBytes(progress.Downloaded)+"/"+FormatBytes(progress.TotalSize),
		FormatBytes(progress.Speed)+"/s",
		progress.RemainingETA.Round(time.Second))
}

// JSONProgress outputs progress as JSON (for scripting)
type JSONProgress struct {
	Filename   string  `json:"filename"`
	Percent    float64 `json:"percent"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Speed      int64   `json:"speed"`
	ETA        int     `json:"eta"`
}

// RenderJSON outputs progress as JSON line
func RenderJSON(w io.Writer, progress engine.Progress, filename string) {
	fmt.Fprintf(w, `{"filename":%q,"percent":%.1f,"downloaded":%d,"total":%d,"speed":%d,"eta":%d}`+"\n",
		filename,
		progress.Percent,
		progress.Downloaded,
		progress.TotalSize,
		progress.Speed,
		int(progress.RemainingETA.Seconds()))
}
