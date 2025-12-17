package hooks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestNewCommandHook(t *testing.T) {
	hook := NewCommandHook("echo test")
	
	if hook.Command != "echo test" {
		t.Errorf("Command = %q, want %q", hook.Command, "echo test")
	}
	
	// Default events should be complete and error
	if len(hook.Events) != 2 {
		t.Errorf("Events len = %d, want 2", len(hook.Events))
	}
}

func TestCommandHook_Name(t *testing.T) {
	hook := NewCommandHook("echo test")
	expected := "command:echo test"
	if hook.Name() != expected {
		t.Errorf("Name() = %q, want %q", hook.Name(), expected)
	}
}

func TestCommandHook_Execute(t *testing.T) {
	// Skip on non-*nix systems for portability
	if isWindows() {
		t.Skip("Skipping on Windows")
	}
	
	hook := NewCommandHook("echo $BURKUT_EVENT", EventComplete)
	
	payload := &Payload{
		Event:      EventComplete,
		URL:        "https://example.com/file.zip",
		Filename:   "file.zip",
		OutputPath: "/tmp/file.zip",
	}
	
	ctx := context.Background()
	err := hook.Execute(ctx, payload)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestCommandHook_Execute_WrongEvent(t *testing.T) {
	hook := NewCommandHook("echo test", EventComplete)
	
	payload := &Payload{
		Event: EventError, // Hook is not configured for this event
	}
	
	ctx := context.Background()
	err := hook.Execute(ctx, payload)
	if err != nil {
		t.Errorf("Execute() should skip non-matching events, got error = %v", err)
	}
}

func TestCommandHook_BuildEnv(t *testing.T) {
	hook := &CommandHook{}
	
	payload := &Payload{
		Event:      EventComplete,
		URL:        "https://example.com/file.zip",
		Filename:   "file.zip",
		OutputPath: "/tmp/file.zip",
		TotalSize:  1000,
		Downloaded: 500,
		Percent:    50.0,
		Speed:      100,
		Error:      "test error",
		Duration:   10.5,
	}
	
	env := hook.buildEnv(payload)
	
	// Check some key environment variables
	expected := map[string]bool{
		"BURKUT_EVENT=complete":     true,
		"BURKUT_FILENAME=file.zip":  true,
		"BURKUT_SIZE=1000":          true,
		"BURKUT_DOWNLOADED=500":     true,
	}
	
	for _, e := range env {
		if expected[e] {
			delete(expected, e)
		}
	}
	
	if len(expected) > 0 {
		t.Errorf("Missing environment variables: %v", expected)
	}
}

func TestNewWebhookHook(t *testing.T) {
	hook := NewWebhookHook("https://example.com/webhook")
	
	if hook.URL != "https://example.com/webhook" {
		t.Errorf("URL = %q", hook.URL)
	}
	
	if len(hook.Events) != 2 {
		t.Errorf("Events len = %d, want 2", len(hook.Events))
	}
}

func TestWebhookHook_WithHeader(t *testing.T) {
	hook := NewWebhookHook("https://example.com/webhook").
		WithHeader("Authorization", "Bearer token123")
	
	if hook.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Header not set correctly")
	}
}

func TestWebhookHook_Execute(t *testing.T) {
	// Create test server
	var receivedPayload Payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s", r.Header.Get("Content-Type"))
		}
		
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&receivedPayload); err != nil {
			t.Errorf("Decode error = %v", err)
		}
		
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	
	hook := NewWebhookHook(server.URL, EventComplete)
	
	payload := &Payload{
		Event:      EventComplete,
		URL:        "https://example.com/file.zip",
		Filename:   "file.zip",
		OutputPath: "/tmp/file.zip",
		TotalSize:  1000,
		Downloaded: 1000,
		Percent:    100.0,
		Timestamp:  time.Now(),
	}
	
	ctx := context.Background()
	err := hook.Execute(ctx, payload)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	
	if receivedPayload.Event != EventComplete {
		t.Errorf("Received event = %s, want complete", receivedPayload.Event)
	}
	
	if receivedPayload.Filename != "file.zip" {
		t.Errorf("Received filename = %s", receivedPayload.Filename)
	}
}

func TestWebhookHook_Execute_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	
	hook := NewWebhookHook(server.URL, EventComplete)
	
	payload := &Payload{Event: EventComplete}
	
	ctx := context.Background()
	err := hook.Execute(ctx, payload)
	if err == nil {
		t.Error("Execute() should return error for 500 response")
	}
}

func TestManager(t *testing.T) {
	manager := NewManager()
	
	if manager.Count() != 0 {
		t.Errorf("Count() = %d, want 0", manager.Count())
	}
	
	manager.AddCommand("echo test", EventComplete)
	manager.AddWebhook("https://example.com/webhook", EventComplete)
	
	if manager.Count() != 2 {
		t.Errorf("Count() = %d, want 2", manager.Count())
	}
	
	manager.Clear()
	
	if manager.Count() != 0 {
		t.Errorf("Count() after Clear() = %d, want 0", manager.Count())
	}
}

func TestManager_Execute(t *testing.T) {
	// Create test server
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	
	manager := NewManager()
	manager.AddWebhook(server.URL, EventComplete)
	
	payload := &Payload{
		Event:    EventComplete,
		Filename: "test.zip",
	}
	
	ctx := context.Background()
	err := manager.Execute(ctx, payload)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestCreatePayload(t *testing.T) {
	payload := CreatePayload(EventStart, "https://example.com/file.zip", "file.zip", "/tmp/file.zip")
	
	if payload.Event != EventStart {
		t.Errorf("Event = %s, want start", payload.Event)
	}
	
	if payload.URL != "https://example.com/file.zip" {
		t.Errorf("URL = %s", payload.URL)
	}
	
	if payload.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestPayload_Fluent(t *testing.T) {
	payload := CreatePayload(EventComplete, "", "", "").
		WithProgress(500, 1000, 100, 50.0).
		WithError(os.ErrNotExist).
		WithDuration(10 * time.Second)
	
	if payload.Downloaded != 500 {
		t.Errorf("Downloaded = %d, want 500", payload.Downloaded)
	}
	
	if payload.TotalSize != 1000 {
		t.Errorf("TotalSize = %d, want 1000", payload.TotalSize)
	}
	
	if payload.Error == "" {
		t.Error("Error should be set")
	}
	
	if payload.Duration != 10.0 {
		t.Errorf("Duration = %f, want 10.0", payload.Duration)
	}
}

func TestIsWindows(t *testing.T) {
	// Just test that it doesn't panic
	result := isWindows()
	_ = result // We can't really test the result portably
}
