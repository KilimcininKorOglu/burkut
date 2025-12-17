// Package tui provides a modern terminal user interface using Bubbletea.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kilimcininkoroglu/burkut/internal/engine"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginBottom(1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	highlightStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	chunkCompleteStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	chunkActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39"))

	chunkPendingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
)

// DownloadState represents the current download state
type DownloadState int

const (
	StateConnecting DownloadState = iota
	StateDownloading
	StatePaused
	StateCompleted
	StateError
	StateVerifying
)

// ChunkInfo represents information about a single chunk
type ChunkInfo struct {
	ID         int
	Start      int64
	End        int64
	Downloaded int64
	Status     string // "pending", "downloading", "completed", "error"
}

// Model is the Bubbletea model for the download TUI
type Model struct {
	// Download info
	URL           string
	Filename      string
	TotalSize     int64
	Downloaded    int64
	Speed         int64
	ETA           time.Duration
	ElapsedTime   time.Duration
	State         DownloadState
	Error         error
	Checksum      string
	ChecksumValid bool

	// Chunks
	Chunks      []ChunkInfo
	Connections int

	// UI components
	progress    progress.Model
	spinner     spinner.Model
	width       int
	height      int
	showChunks  bool
	quitting    bool

	// Callbacks
	onPause  func()
	onResume func()
	onCancel func()
}

// ProgressMsg is sent when download progress updates
type ProgressMsg struct {
	Progress engine.Progress
	Chunks   []ChunkInfo
}

// CompleteMsg is sent when download completes
type CompleteMsg struct {
	Filename string
	Size     int64
	Duration time.Duration
	Speed    int64
}

// ErrorMsg is sent when an error occurs
type ErrorMsg struct {
	Err error
}

// VerifyingMsg is sent when checksum verification starts
type VerifyingMsg struct{}

// VerifiedMsg is sent when checksum verification completes
type VerifiedMsg struct {
	Valid bool
}

// NewModel creates a new TUI model
func NewModel(url, filename string, totalSize int64, connections int) Model {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(50),
		progress.WithoutPercentage(),
	)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	chunks := make([]ChunkInfo, connections)
	for i := range chunks {
		chunks[i] = ChunkInfo{
			ID:     i,
			Status: "pending",
		}
	}

	return Model{
		URL:         url,
		Filename:    filename,
		TotalSize:   totalSize,
		Connections: connections,
		Chunks:      chunks,
		State:       StateConnecting,
		progress:    p,
		spinner:     s,
		showChunks:  true,
		width:       80,
		height:      24,
	}
}

// SetCallbacks sets the control callbacks
func (m *Model) SetCallbacks(onPause, onResume, onCancel func()) {
	m.onPause = onPause
	m.onResume = onResume
	m.onCancel = onCancel
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tea.EnterAltScreen,
	)
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			if m.onCancel != nil {
				m.onCancel()
			}
			return m, tea.Quit

		case "p", " ":
			if m.State == StateDownloading {
				m.State = StatePaused
				if m.onPause != nil {
					m.onPause()
				}
			} else if m.State == StatePaused {
				m.State = StateDownloading
				if m.onResume != nil {
					m.onResume()
				}
			}

		case "c":
			m.showChunks = !m.showChunks
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 10
		if m.progress.Width > 80 {
			m.progress.Width = 80
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case ProgressMsg:
		m.State = StateDownloading
		m.Downloaded = msg.Progress.Downloaded
		m.Speed = msg.Progress.Speed
		m.ETA = msg.Progress.ETA
		m.ElapsedTime = msg.Progress.ElapsedTime
		if len(msg.Chunks) > 0 {
			m.Chunks = msg.Chunks
		}
		return m, nil

	case CompleteMsg:
		m.State = StateCompleted
		m.Downloaded = msg.Size
		m.ElapsedTime = msg.Duration
		m.Speed = msg.Speed
		return m, nil

	case ErrorMsg:
		m.State = StateError
		m.Error = msg.Err
		return m, nil

	case VerifyingMsg:
		m.State = StateVerifying
		return m, nil

	case VerifiedMsg:
		m.ChecksumValid = msg.Valid
		return m, nil
	}

	return m, nil
}

// View implements tea.Model
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render("🦅 Burkut Download Manager"))
	b.WriteString("\n\n")

	// File info
	b.WriteString(m.renderFileInfo())
	b.WriteString("\n\n")

	// Progress
	b.WriteString(m.renderProgress())
	b.WriteString("\n\n")

	// Stats
	b.WriteString(m.renderStats())
	b.WriteString("\n")

	// Chunks (optional)
	if m.showChunks && m.State == StateDownloading {
		b.WriteString("\n")
		b.WriteString(m.renderChunks())
		b.WriteString("\n")
	}

	// Status/Error message
	b.WriteString("\n")
	b.WriteString(m.renderStatus())

	// Help
	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m Model) renderFileInfo() string {
	var b strings.Builder

	// Filename
	b.WriteString(highlightStyle.Render("File: "))
	b.WriteString(m.Filename)
	b.WriteString("\n")

	// URL (truncated if too long)
	url := m.URL
	if len(url) > 60 {
		url = url[:57] + "..."
	}
	b.WriteString(dimStyle.Render("URL:  "))
	b.WriteString(dimStyle.Render(url))

	return b.String()
}

func (m Model) renderProgress() string {
	var b strings.Builder

	// Calculate percentage
	percent := 0.0
	if m.TotalSize > 0 {
		percent = float64(m.Downloaded) / float64(m.TotalSize)
	}

	// Progress bar
	b.WriteString(m.progress.ViewAs(percent))
	b.WriteString("  ")

	// Percentage
	b.WriteString(highlightStyle.Render(fmt.Sprintf("%.1f%%", percent*100)))

	return b.String()
}

func (m Model) renderStats() string {
	var parts []string

	// Downloaded / Total
	downloaded := formatBytes(m.Downloaded)
	total := formatBytes(m.TotalSize)
	parts = append(parts, fmt.Sprintf("%s / %s", downloaded, total))

	// Speed
	if m.Speed > 0 {
		speed := formatBytes(m.Speed) + "/s"
		parts = append(parts, highlightStyle.Render(speed))
	}

	// ETA
	if m.State == StateDownloading && m.ETA > 0 {
		eta := formatDuration(m.ETA)
		parts = append(parts, fmt.Sprintf("ETA: %s", eta))
	}

	// Elapsed
	if m.ElapsedTime > 0 {
		elapsed := formatDuration(m.ElapsedTime)
		parts = append(parts, dimStyle.Render(fmt.Sprintf("Elapsed: %s", elapsed)))
	}

	return strings.Join(parts, "  │  ")
}

func (m Model) renderChunks() string {
	var b strings.Builder

	b.WriteString(dimStyle.Render("Chunks:"))
	b.WriteString("\n")

	chunksPerRow := 4
	if m.width < 60 {
		chunksPerRow = 2
	}

	for i, chunk := range m.Chunks {
		if i > 0 && i%chunksPerRow == 0 {
			b.WriteString("\n")
		}

		// Calculate chunk progress
		chunkSize := chunk.End - chunk.Start + 1
		chunkPercent := 0.0
		if chunkSize > 0 {
			chunkPercent = float64(chunk.Downloaded) / float64(chunkSize) * 100
		}

		// Chunk indicator
		var indicator string
		var style lipgloss.Style

		switch chunk.Status {
		case "completed":
			indicator = "✓"
			style = chunkCompleteStyle
		case "downloading":
			indicator = "↓"
			style = chunkActiveStyle
		case "error":
			indicator = "✗"
			style = errorStyle
		default:
			indicator = "○"
			style = chunkPendingStyle
		}

		chunkStr := fmt.Sprintf("[%d: %s %5.1f%%]", chunk.ID, indicator, chunkPercent)
		b.WriteString(style.Render(chunkStr))
		b.WriteString("  ")
	}

	return b.String()
}

func (m Model) renderStatus() string {
	switch m.State {
	case StateConnecting:
		return m.spinner.View() + " Connecting..."

	case StateDownloading:
		return successStyle.Render("● Downloading")

	case StatePaused:
		return warningStyle.Render("⏸ Paused")

	case StateVerifying:
		return m.spinner.View() + " Verifying checksum..."

	case StateCompleted:
		msg := successStyle.Render("✓ Download complete!")
		if m.Checksum != "" {
			if m.ChecksumValid {
				msg += "  " + successStyle.Render("Checksum verified ✓")
			} else {
				msg += "  " + errorStyle.Render("Checksum mismatch ✗")
			}
		}
		return msg

	case StateError:
		if m.Error != nil {
			return errorStyle.Render(fmt.Sprintf("✗ Error: %v", m.Error))
		}
		return errorStyle.Render("✗ Download failed")

	default:
		return ""
	}
}

func (m Model) renderHelp() string {
	var keys []string

	if m.State == StateDownloading {
		keys = append(keys, "p:pause")
	} else if m.State == StatePaused {
		keys = append(keys, "p:resume")
	}

	keys = append(keys, "c:toggle chunks")
	keys = append(keys, "q:quit")

	help := strings.Join(keys, " • ")
	return dimStyle.Render(help)
}

// Helper functions

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}
