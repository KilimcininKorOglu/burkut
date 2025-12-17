package engine

import (
	"context"
	"testing"
	"time"
)

func TestNewMirrorList(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	if ml.Count() != 0 {
		t.Errorf("Count() = %d, want 0", ml.Count())
	}
}

func TestMirrorList_Add(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	ml.Add("https://mirror1.example.com/file.zip")
	ml.Add("https://mirror2.example.com/file.zip")

	if ml.Count() != 2 {
		t.Errorf("Count() = %d, want 2", ml.Count())
	}
}

func TestMirrorList_AddWithPriority(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	ml.AddWithPriority("https://mirror1.example.com/file.zip", 10)
	ml.AddWithPriority("https://mirror2.example.com/file.zip", 20)

	mirrors := ml.GetAll()
	if mirrors[0].Priority != 10 {
		t.Errorf("First mirror priority = %d, want 10", mirrors[0].Priority)
	}
	if mirrors[1].Priority != 20 {
		t.Errorf("Second mirror priority = %d, want 20", mirrors[1].Priority)
	}
}

func TestMirrorList_AddMultiple(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	urls := []string{
		"https://mirror1.example.com/file.zip",
		"https://mirror2.example.com/file.zip",
		"https://mirror3.example.com/file.zip",
	}

	ml.AddMultiple(urls)

	if ml.Count() != 3 {
		t.Errorf("Count() = %d, want 3", ml.Count())
	}
}

func TestMirrorList_Next_Failover(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	ml.AddWithPriority("https://low.example.com/file.zip", 1)
	ml.AddWithPriority("https://high.example.com/file.zip", 10)
	ml.AddWithPriority("https://medium.example.com/file.zip", 5)

	// Should return highest priority
	mirror := ml.Next()
	if mirror.URL != "https://high.example.com/file.zip" {
		t.Errorf("Next() URL = %s, want high priority", mirror.URL)
	}
}

func TestMirrorList_Next_RoundRobin(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyRoundRobin)

	ml.Add("https://mirror1.example.com/file.zip")
	ml.Add("https://mirror2.example.com/file.zip")
	ml.Add("https://mirror3.example.com/file.zip")

	// Should cycle through mirrors
	urls := make([]string, 6)
	for i := 0; i < 6; i++ {
		urls[i] = ml.Next().URL
	}

	// First three should be in order
	if urls[0] != "https://mirror1.example.com/file.zip" {
		t.Errorf("urls[0] = %s", urls[0])
	}
	if urls[1] != "https://mirror2.example.com/file.zip" {
		t.Errorf("urls[1] = %s", urls[1])
	}
	if urls[2] != "https://mirror3.example.com/file.zip" {
		t.Errorf("urls[2] = %s", urls[2])
	}

	// Should repeat
	if urls[3] != urls[0] {
		t.Errorf("Round robin should repeat: urls[3]=%s, urls[0]=%s", urls[3], urls[0])
	}
}

func TestMirrorList_Next_Random(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyRandom)

	ml.Add("https://mirror1.example.com/file.zip")
	ml.Add("https://mirror2.example.com/file.zip")
	ml.Add("https://mirror3.example.com/file.zip")

	// Should return a valid mirror
	mirror := ml.Next()
	if mirror == nil {
		t.Error("Next() returned nil")
	}

	// Just verify it's one of our mirrors
	found := false
	for _, m := range ml.GetAll() {
		if m.URL == mirror.URL {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Next() returned unknown mirror: %s", mirror.URL)
	}
}

func TestMirrorList_Next_Fastest(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFastest)

	ml.Add("https://slow.example.com/file.zip")
	ml.Add("https://fast.example.com/file.zip")
	ml.Add("https://medium.example.com/file.zip")

	// Set latencies
	ml.MarkSuccess("https://slow.example.com/file.zip", 500*time.Millisecond)
	ml.MarkSuccess("https://fast.example.com/file.zip", 50*time.Millisecond)
	ml.MarkSuccess("https://medium.example.com/file.zip", 200*time.Millisecond)

	mirror := ml.Next()
	if mirror.URL != "https://fast.example.com/file.zip" {
		t.Errorf("Next() URL = %s, want fastest", mirror.URL)
	}
}

func TestMirrorList_MarkFailed(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	ml.Add("https://mirror1.example.com/file.zip")

	// Should be healthy initially
	if ml.HealthyCount() != 1 {
		t.Errorf("HealthyCount() = %d, want 1", ml.HealthyCount())
	}

	// Mark failed 3 times
	ml.MarkFailed("https://mirror1.example.com/file.zip")
	ml.MarkFailed("https://mirror1.example.com/file.zip")
	ml.MarkFailed("https://mirror1.example.com/file.zip")

	// Should be unhealthy now
	if ml.HealthyCount() != 0 {
		t.Errorf("HealthyCount() after failures = %d, want 0", ml.HealthyCount())
	}
}

func TestMirrorList_MarkSuccess(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	ml.Add("https://mirror1.example.com/file.zip")

	// Mark failed then success
	ml.MarkFailed("https://mirror1.example.com/file.zip")
	ml.MarkFailed("https://mirror1.example.com/file.zip")
	ml.MarkFailed("https://mirror1.example.com/file.zip")
	ml.MarkSuccess("https://mirror1.example.com/file.zip", 100*time.Millisecond)

	if ml.HealthyCount() != 1 {
		t.Errorf("HealthyCount() after success = %d, want 1", ml.HealthyCount())
	}

	mirrors := ml.GetAll()
	if mirrors[0].FailCount != 0 {
		t.Errorf("FailCount after success = %d, want 0", mirrors[0].FailCount)
	}
}

func TestMirrorList_Next_Empty(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	mirror := ml.Next()
	if mirror != nil {
		t.Error("Next() should return nil for empty list")
	}
}

func TestMirrorList_AllUnhealthy_Reset(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	ml.Add("https://mirror1.example.com/file.zip")

	// Mark as unhealthy
	for i := 0; i < 3; i++ {
		ml.MarkFailed("https://mirror1.example.com/file.zip")
	}

	// Next should reset health and return the mirror
	mirror := ml.Next()
	if mirror == nil {
		t.Error("Next() should reset and return mirror when all unhealthy")
	}
}

func TestNewMirrorDownloader(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)
	ml.Add("https://example.com/file.zip")

	// Create a basic downloader (we won't actually download)
	cfg := DefaultConfig()
	md := NewMirrorDownloader(NewDownloader(cfg, nil), ml)

	if md == nil {
		t.Error("NewMirrorDownloader returned nil")
	}
}

func TestMirrorDownloader_Download_NoMirrors(t *testing.T) {
	ml := NewMirrorList(MirrorStrategyFailover)

	cfg := DefaultConfig()
	md := NewMirrorDownloader(NewDownloader(cfg, nil), ml)

	ctx := context.Background()
	err := md.Download(ctx, "/tmp/test.zip")

	if err == nil {
		t.Error("Download() should fail with no mirrors")
	}
}

func TestParseMirrorFile(t *testing.T) {
	content := `# This is a comment
https://mirror1.example.com/file.zip
https://mirror2.example.com/file.zip

# Another comment
https://mirror3.example.com/file.zip
`

	mirrors := ParseMirrorFile(content)

	if len(mirrors) != 3 {
		t.Errorf("ParseMirrorFile() len = %d, want 3", len(mirrors))
	}

	expected := []string{
		"https://mirror1.example.com/file.zip",
		"https://mirror2.example.com/file.zip",
		"https://mirror3.example.com/file.zip",
	}

	for i, exp := range expected {
		if mirrors[i] != exp {
			t.Errorf("mirrors[%d] = %q, want %q", i, mirrors[i], exp)
		}
	}
}

func TestParseMirrorFile_Empty(t *testing.T) {
	mirrors := ParseMirrorFile("")
	if len(mirrors) != 0 {
		t.Errorf("ParseMirrorFile(\"\") len = %d, want 0", len(mirrors))
	}
}

func TestParseMirrorFile_OnlyComments(t *testing.T) {
	content := `# Comment 1
# Comment 2
`
	mirrors := ParseMirrorFile(content)
	if len(mirrors) != 0 {
		t.Errorf("ParseMirrorFile() len = %d, want 0", len(mirrors))
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"line1", 1},
		{"line1\nline2", 2},
		{"line1\r\nline2", 2},
		{"line1\nline2\n", 2},
	}

	for _, tt := range tests {
		lines := splitLines(tt.input)
		if len(lines) != tt.expected {
			t.Errorf("splitLines(%q) len = %d, want %d", tt.input, len(lines), tt.expected)
		}
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"hello", "hello"},
		{"  hello", "hello"},
		{"hello  ", "hello"},
		{"  hello  ", "hello"},
		{"\thello\t", "hello"},
	}

	for _, tt := range tests {
		result := trimSpace(tt.input)
		if result != tt.expected {
			t.Errorf("trimSpace(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
