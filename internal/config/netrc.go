// Package config provides configuration management for Burkut.
package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// NetrcEntry represents a single entry in a netrc file
type NetrcEntry struct {
	Machine  string
	Login    string
	Password string
	Account  string
}

// Netrc represents a parsed netrc file
type Netrc struct {
	entries map[string]*NetrcEntry
	Default *NetrcEntry
}

// NetrcPath returns the default netrc file path based on OS
func NetrcPath() string {
	if runtime.GOOS == "windows" {
		// Windows: %USERPROFILE%\_netrc
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, "_netrc")
	}

	// Unix: ~/.netrc
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".netrc")
}

// ParseNetrc parses a netrc file from the given path
func ParseNetrc(path string) (*Netrc, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening netrc file: %w", err)
	}
	defer file.Close()

	netrc := &Netrc{
		entries: make(map[string]*NetrcEntry),
	}

	var currentEntry *NetrcEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Tokenize the line
		tokens := tokenizeLine(line)

		for i := 0; i < len(tokens); i++ {
			token := strings.ToLower(tokens[i])

			switch token {
			case "machine":
				if i+1 < len(tokens) {
					i++
					currentEntry = &NetrcEntry{Machine: tokens[i]}
					netrc.entries[currentEntry.Machine] = currentEntry
				}

			case "default":
				currentEntry = &NetrcEntry{Machine: ""}
				netrc.Default = currentEntry

			case "login":
				if currentEntry != nil && i+1 < len(tokens) {
					i++
					currentEntry.Login = tokens[i]
				}

			case "password":
				if currentEntry != nil && i+1 < len(tokens) {
					i++
					currentEntry.Password = tokens[i]
				}

			case "account":
				if currentEntry != nil && i+1 < len(tokens) {
					i++
					currentEntry.Account = tokens[i]
				}

			case "macdef":
				// Skip macro definitions - read until empty line
				for scanner.Scan() {
					if strings.TrimSpace(scanner.Text()) == "" {
						break
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading netrc file: %w", err)
	}

	return netrc, nil
}

// tokenizeLine splits a line into tokens, handling quoted values
func tokenizeLine(line string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if inQuote {
			if ch == quoteChar {
				inQuote = false
				// Add the token without quotes
				tokens = append(tokens, current.String())
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		} else {
			switch ch {
			case '"', '\'':
				inQuote = true
				quoteChar = ch
			case ' ', '\t':
				if current.Len() > 0 {
					tokens = append(tokens, current.String())
					current.Reset()
				}
			default:
				current.WriteByte(ch)
			}
		}
	}

	// Add last token if any
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// LoadNetrc loads and parses the default netrc file
func LoadNetrc() (*Netrc, error) {
	path := NetrcPath()
	if path == "" {
		return nil, fmt.Errorf("cannot determine netrc path")
	}

	return ParseNetrc(path)
}

// FindEntry finds a netrc entry for the given host
func (n *Netrc) FindEntry(host string) *NetrcEntry {
	if n == nil {
		return nil
	}

	// Try exact match first
	if entry, ok := n.entries[host]; ok {
		return entry
	}

	// Try lowercase
	if entry, ok := n.entries[strings.ToLower(host)]; ok {
		return entry
	}

	// Return default if available
	return n.Default
}

// FindEntryForURL finds a netrc entry for the given URL
func (n *Netrc) FindEntryForURL(rawURL string) *NetrcEntry {
	if n == nil {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	return n.FindEntry(parsed.Hostname())
}

// GetCredentials returns login and password for a URL
func (n *Netrc) GetCredentials(rawURL string) (login, password string, found bool) {
	entry := n.FindEntryForURL(rawURL)
	if entry == nil {
		return "", "", false
	}

	return entry.Login, entry.Password, true
}

// String returns a string representation of the netrc (for debugging)
func (n *Netrc) String() string {
	if n == nil {
		return "<nil>"
	}

	var sb strings.Builder
	sb.WriteString("Netrc{\n")

	for host, entry := range n.entries {
		sb.WriteString(fmt.Sprintf("  machine %s login %s password ***\n", host, entry.Login))
	}

	if n.Default != nil {
		sb.WriteString(fmt.Sprintf("  default login %s password ***\n", n.Default.Login))
	}

	sb.WriteString("}")
	return sb.String()
}

// HasEntries returns true if the netrc has any entries
func (n *Netrc) HasEntries() bool {
	if n == nil {
		return false
	}
	return len(n.entries) > 0 || n.Default != nil
}
