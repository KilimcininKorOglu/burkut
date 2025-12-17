package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseNetrc(t *testing.T) {
	tmpDir := t.TempDir()
	netrcFile := filepath.Join(tmpDir, ".netrc")

	content := `# This is a comment
machine example.com
	login user1
	password pass1

machine api.github.com login user2 password pass2

machine secure.example.org
login user3
password "pass with spaces"

default login defaultuser password defaultpass
`

	if err := os.WriteFile(netrcFile, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	netrc, err := ParseNetrc(netrcFile)
	if err != nil {
		t.Fatalf("ParseNetrc() error = %v", err)
	}

	tests := []struct {
		host         string
		wantLogin    string
		wantPassword string
	}{
		{"example.com", "user1", "pass1"},
		{"api.github.com", "user2", "pass2"},
		{"secure.example.org", "user3", "pass with spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			entry := netrc.FindEntry(tt.host)
			if entry == nil {
				t.Fatalf("FindEntry(%s) returned nil", tt.host)
			}

			if entry.Login != tt.wantLogin {
				t.Errorf("Login = %s, want %s", entry.Login, tt.wantLogin)
			}

			if entry.Password != tt.wantPassword {
				t.Errorf("Password = %s, want %s", entry.Password, tt.wantPassword)
			}
		})
	}

	// Test default entry
	if netrc.Default == nil {
		t.Fatal("Default entry should not be nil")
	}

	if netrc.Default.Login != "defaultuser" {
		t.Errorf("Default.Login = %s, want defaultuser", netrc.Default.Login)
	}

	// Test unknown host falls back to default
	entry := netrc.FindEntry("unknown.host")
	if entry != netrc.Default {
		t.Error("Unknown host should return default entry")
	}
}

func TestNetrc_FindEntryForURL(t *testing.T) {
	tmpDir := t.TempDir()
	netrcFile := filepath.Join(tmpDir, ".netrc")

	content := `machine github.com login ghuser password ghpass
machine example.com login exuser password expass
`

	if err := os.WriteFile(netrcFile, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	netrc, err := ParseNetrc(netrcFile)
	if err != nil {
		t.Fatalf("ParseNetrc() error = %v", err)
	}

	tests := []struct {
		url       string
		wantLogin string
		wantFound bool
	}{
		{"https://github.com/user/repo", "ghuser", true},
		{"https://example.com/path/to/file.zip", "exuser", true},
		{"http://github.com:443/other", "ghuser", true},
		{"https://unknown.com/file", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			login, _, found := netrc.GetCredentials(tt.url)

			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}

			if found && login != tt.wantLogin {
				t.Errorf("login = %s, want %s", login, tt.wantLogin)
			}
		})
	}
}

func TestNetrc_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	netrcFile := filepath.Join(tmpDir, ".netrc")

	if err := os.WriteFile(netrcFile, []byte(""), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	netrc, err := ParseNetrc(netrcFile)
	if err != nil {
		t.Fatalf("ParseNetrc() error = %v", err)
	}

	if netrc.HasEntries() {
		t.Error("Empty netrc should have no entries")
	}
}

func TestNetrc_NonExistent(t *testing.T) {
	_, err := ParseNetrc("/nonexistent/path/.netrc")
	if err == nil {
		t.Error("ParseNetrc() should return error for non-existent file")
	}
}

func TestNetrc_SingleLine(t *testing.T) {
	tmpDir := t.TempDir()
	netrcFile := filepath.Join(tmpDir, ".netrc")

	// All on single line
	content := `machine test.com login testuser password testpass`

	if err := os.WriteFile(netrcFile, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	netrc, err := ParseNetrc(netrcFile)
	if err != nil {
		t.Fatalf("ParseNetrc() error = %v", err)
	}

	entry := netrc.FindEntry("test.com")
	if entry == nil {
		t.Fatal("Entry should not be nil")
	}

	if entry.Login != "testuser" {
		t.Errorf("Login = %s, want testuser", entry.Login)
	}

	if entry.Password != "testpass" {
		t.Errorf("Password = %s, want testpass", entry.Password)
	}
}

func TestTokenizeLine(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"machine example.com", []string{"machine", "example.com"}},
		{"login user password pass", []string{"login", "user", "password", "pass"}},
		{`password "pass with spaces"`, []string{"password", "pass with spaces"}},
		{`login 'quoted user'`, []string{"login", "quoted user"}},
		{"  extra   whitespace  ", []string{"extra", "whitespace"}},
		{"", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tokenizeLine(tt.input)

			if len(got) != len(tt.expected) {
				t.Errorf("len = %d, want %d", len(got), len(tt.expected))
				return
			}

			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("token[%d] = %s, want %s", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestNetrcPath(t *testing.T) {
	path := NetrcPath()
	if path == "" {
		t.Skip("Cannot determine netrc path (no home directory)")
	}

	// Just verify it returns something reasonable
	if !filepath.IsAbs(path) {
		t.Error("NetrcPath should return absolute path")
	}
}
