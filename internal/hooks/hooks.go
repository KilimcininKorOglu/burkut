// Package hooks provides event hooks for download lifecycle events.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Event represents a download lifecycle event
type Event string

const (
	EventStart    Event = "start"    // Download started
	EventProgress Event = "progress" // Progress update
	EventComplete Event = "complete" // Download completed successfully
	EventError    Event = "error"    // Download failed
	EventCancel   Event = "cancel"   // Download cancelled
)

// Payload contains information about the download event
type Payload struct {
	Event      Event     `json:"event"`
	URL        string    `json:"url"`
	Filename   string    `json:"filename"`
	OutputPath string    `json:"output_path"`
	TotalSize  int64     `json:"total_size"`
	Downloaded int64     `json:"downloaded"`
	Percent    float64   `json:"percent"`
	Speed      int64     `json:"speed"`
	Error      string    `json:"error,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Duration   float64   `json:"duration_seconds,omitempty"`
}

// Hook is the interface for all hook types
type Hook interface {
	Execute(ctx context.Context, payload *Payload) error
	Name() string
}

// CommandHook executes a shell command on events
type CommandHook struct {
	Command string
	Events  []Event
	Timeout time.Duration
}

// NewCommandHook creates a new command hook
func NewCommandHook(command string, events ...Event) *CommandHook {
	if len(events) == 0 {
		events = []Event{EventComplete, EventError}
	}
	return &CommandHook{
		Command: command,
		Events:  events,
		Timeout: 30 * time.Second,
	}
}

// Name returns the hook name
func (h *CommandHook) Name() string {
	return fmt.Sprintf("command:%s", h.Command)
}

// Execute runs the command with environment variables set
func (h *CommandHook) Execute(ctx context.Context, payload *Payload) error {
	// Check if this event is in our list
	if !h.shouldHandle(payload.Event) {
		return nil
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()

	// Determine shell based on OS
	var cmd *exec.Cmd
	if isWindows() {
		cmd = exec.CommandContext(ctx, "cmd", "/C", h.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", h.Command)
	}

	// Set environment variables
	cmd.Env = append(os.Environ(), h.buildEnv(payload)...)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("hook command failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func (h *CommandHook) shouldHandle(event Event) bool {
	for _, e := range h.Events {
		if e == event {
			return true
		}
	}
	return false
}

func (h *CommandHook) buildEnv(payload *Payload) []string {
	return []string{
		fmt.Sprintf("BURKUT_EVENT=%s", payload.Event),
		fmt.Sprintf("BURKUT_URL=%s", payload.URL),
		fmt.Sprintf("BURKUT_FILENAME=%s", payload.Filename),
		fmt.Sprintf("BURKUT_OUTPUT=%s", payload.OutputPath),
		fmt.Sprintf("BURKUT_SIZE=%d", payload.TotalSize),
		fmt.Sprintf("BURKUT_DOWNLOADED=%d", payload.Downloaded),
		fmt.Sprintf("BURKUT_PERCENT=%.2f", payload.Percent),
		fmt.Sprintf("BURKUT_SPEED=%d", payload.Speed),
		fmt.Sprintf("BURKUT_ERROR=%s", payload.Error),
		fmt.Sprintf("BURKUT_DURATION=%.2f", payload.Duration),
	}
}

// WebhookHook sends HTTP POST requests on events
type WebhookHook struct {
	URL     string
	Events  []Event
	Headers map[string]string
	Timeout time.Duration
	client  *http.Client
}

// NewWebhookHook creates a new webhook hook
func NewWebhookHook(url string, events ...Event) *WebhookHook {
	if len(events) == 0 {
		events = []Event{EventComplete, EventError}
	}
	return &WebhookHook{
		URL:     url,
		Events:  events,
		Headers: make(map[string]string),
		Timeout: 10 * time.Second,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// WithHeader adds a header to the webhook request
func (h *WebhookHook) WithHeader(key, value string) *WebhookHook {
	h.Headers[key] = value
	return h
}

// Name returns the hook name
func (h *WebhookHook) Name() string {
	return fmt.Sprintf("webhook:%s", h.URL)
}

// Execute sends the webhook request
func (h *WebhookHook) Execute(ctx context.Context, payload *Payload) error {
	// Check if this event is in our list
	if !h.shouldHandle(payload.Event) {
		return nil
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()

	// Marshal payload to JSON
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Burkut-Webhook/1.0")
	for key, value := range h.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (h *WebhookHook) shouldHandle(event Event) bool {
	for _, e := range h.Events {
		if e == event {
			return true
		}
	}
	return false
}

// Manager manages multiple hooks
type Manager struct {
	hooks []Hook
}

// NewManager creates a new hook manager
func NewManager() *Manager {
	return &Manager{
		hooks: make([]Hook, 0),
	}
}

// Add adds a hook to the manager
func (m *Manager) Add(hook Hook) {
	m.hooks = append(m.hooks, hook)
}

// AddCommand adds a command hook
func (m *Manager) AddCommand(command string, events ...Event) {
	m.Add(NewCommandHook(command, events...))
}

// AddWebhook adds a webhook hook
func (m *Manager) AddWebhook(url string, events ...Event) {
	m.Add(NewWebhookHook(url, events...))
}

// Execute runs all hooks for the given event
func (m *Manager) Execute(ctx context.Context, payload *Payload) error {
	var errors []string

	for _, hook := range m.hooks {
		if err := hook.Execute(ctx, payload); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", hook.Name(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("hook errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// ExecuteAsync runs all hooks asynchronously (fire and forget)
func (m *Manager) ExecuteAsync(ctx context.Context, payload *Payload) {
	for _, hook := range m.hooks {
		go func(h Hook) {
			_ = h.Execute(ctx, payload)
		}(hook)
	}
}

// Count returns the number of registered hooks
func (m *Manager) Count() int {
	return len(m.hooks)
}

// Clear removes all hooks
func (m *Manager) Clear() {
	m.hooks = make([]Hook, 0)
}

// isWindows returns true if running on Windows
func isWindows() bool {
	return strings.Contains(strings.ToLower(os.Getenv("OS")), "windows")
}

// CreatePayload creates a new payload for an event
func CreatePayload(event Event, url, filename, outputPath string) *Payload {
	return &Payload{
		Event:      event,
		URL:        url,
		Filename:   filename,
		OutputPath: outputPath,
		Timestamp:  time.Now(),
	}
}

// WithProgress adds progress information to the payload
func (p *Payload) WithProgress(downloaded, totalSize, speed int64, percent float64) *Payload {
	p.Downloaded = downloaded
	p.TotalSize = totalSize
	p.Speed = speed
	p.Percent = percent
	return p
}

// WithError adds error information to the payload
func (p *Payload) WithError(err error) *Payload {
	if err != nil {
		p.Error = err.Error()
	}
	return p
}

// WithDuration adds duration information to the payload
func (p *Payload) WithDuration(d time.Duration) *Payload {
	p.Duration = d.Seconds()
	return p
}
