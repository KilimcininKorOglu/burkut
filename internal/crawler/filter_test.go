package crawler

import (
	"net/url"
	"testing"
)

func TestNewFilter(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	f := NewFilter(baseURL)

	if f == nil {
		t.Fatal("NewFilter returned nil")
	}
	if !f.SameDomain {
		t.Error("SameDomain should be true by default")
	}
	if f.FollowExternal {
		t.Error("FollowExternal should be false by default")
	}
	if !f.PageRequisites {
		t.Error("PageRequisites should be true by default")
	}
}

func TestFilter_ShouldCrawl_SameDomain(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	f := NewFilter(baseURL)

	tests := []struct {
		urlStr   string
		expected bool
	}{
		{"https://example.com/page1", true},
		{"https://example.com/dir/page2", true},
		{"https://sub.example.com/page", true}, // Subdomain
		{"https://other.com/page", false},
		{"http://example.com/page", true}, // Different scheme OK
		{"ftp://example.com/file", false}, // Non-HTTP
	}

	for _, tt := range tests {
		t.Run(tt.urlStr, func(t *testing.T) {
			u, _ := url.Parse(tt.urlStr)
			result := f.ShouldCrawl(u, LinkTypeAnchor)
			if result != tt.expected {
				t.Errorf("ShouldCrawl(%s) = %v, want %v", tt.urlStr, result, tt.expected)
			}
		})
	}
}

func TestFilter_ShouldCrawl_AcceptPatterns(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	f := NewFilter(baseURL)
	f.SetAcceptPatterns("*.pdf,*.doc")

	tests := []struct {
		urlStr   string
		expected bool
	}{
		{"https://example.com/file.pdf", true},
		{"https://example.com/file.doc", true},
		{"https://example.com/file.txt", false},
		{"https://example.com/document.PDF", true}, // Case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.urlStr, func(t *testing.T) {
			u, _ := url.Parse(tt.urlStr)
			result := f.ShouldCrawl(u, LinkTypeAnchor)
			if result != tt.expected {
				t.Errorf("ShouldCrawl(%s) = %v, want %v", tt.urlStr, result, tt.expected)
			}
		})
	}
}

func TestFilter_ShouldCrawl_RejectPatterns(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	f := NewFilter(baseURL)
	f.SetRejectPatterns("*.exe,*.zip")

	tests := []struct {
		urlStr   string
		expected bool
	}{
		{"https://example.com/file.pdf", true},
		{"https://example.com/file.exe", false},
		{"https://example.com/archive.zip", false},
	}

	for _, tt := range tests {
		t.Run(tt.urlStr, func(t *testing.T) {
			u, _ := url.Parse(tt.urlStr)
			result := f.ShouldCrawl(u, LinkTypeAnchor)
			if result != tt.expected {
				t.Errorf("ShouldCrawl(%s) = %v, want %v", tt.urlStr, result, tt.expected)
			}
		})
	}
}

func TestFilter_ShouldCrawl_Extensions(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	f := NewFilter(baseURL)
	f.SetAcceptExtensions("html,htm,php")
	f.SetRejectExtensions("exe")

	tests := []struct {
		urlStr   string
		expected bool
	}{
		{"https://example.com/page.html", true},
		{"https://example.com/page.htm", true},
		{"https://example.com/page.php", true},
		{"https://example.com/page.asp", false}, // Not in accept list
		{"https://example.com/file.exe", false}, // Rejected
	}

	for _, tt := range tests {
		t.Run(tt.urlStr, func(t *testing.T) {
			u, _ := url.Parse(tt.urlStr)
			result := f.ShouldCrawl(u, LinkTypeAnchor)
			if result != tt.expected {
				t.Errorf("ShouldCrawl(%s) = %v, want %v", tt.urlStr, result, tt.expected)
			}
		})
	}
}

func TestFilter_PageRequisites(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	f := NewFilter(baseURL)
	f.PageRequisites = true

	// External image should be allowed as page requisite
	u, _ := url.Parse("https://cdn.other.com/image.png")
	if !f.ShouldCrawl(u, LinkTypeImage) {
		t.Error("External image should be allowed when PageRequisites is true")
	}

	// But external anchor should not
	u, _ = url.Parse("https://other.com/page")
	if f.ShouldCrawl(u, LinkTypeAnchor) {
		t.Error("External anchor should not be allowed")
	}
}

func TestFilter_AllowedDomains(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	f := NewFilter(baseURL)
	f.SameDomain = false
	f.AllowedDomains = []string{"example.com", "*.cdn.example.com"}

	tests := []struct {
		urlStr   string
		expected bool
	}{
		{"https://example.com/page", true},
		{"https://img.cdn.example.com/pic.png", true},
		{"https://other.com/page", false},
	}

	for _, tt := range tests {
		t.Run(tt.urlStr, func(t *testing.T) {
			u, _ := url.Parse(tt.urlStr)
			result := f.ShouldCrawl(u, LinkTypeAnchor)
			if result != tt.expected {
				t.Errorf("ShouldCrawl(%s) = %v, want %v", tt.urlStr, result, tt.expected)
			}
		})
	}
}

func TestFilter_ExcludedDomains(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	f := NewFilter(baseURL)
	f.SameDomain = false
	f.ExcludedDomains = []string{"ads.example.com", "*.tracking.com"}

	tests := []struct {
		urlStr   string
		expected bool
	}{
		{"https://example.com/page", true},
		{"https://ads.example.com/banner", false},
		{"https://google.tracking.com/pixel", false},
		{"https://other.com/page", true},
	}

	for _, tt := range tests {
		t.Run(tt.urlStr, func(t *testing.T) {
			u, _ := url.Parse(tt.urlStr)
			result := f.ShouldCrawl(u, LinkTypeAnchor)
			if result != tt.expected {
				t.Errorf("ShouldCrawl(%s) = %v, want %v", tt.urlStr, result, tt.expected)
			}
		})
	}
}

func TestSimpleGlobMatch(t *testing.T) {
	tests := []struct {
		str      string
		pattern  string
		expected bool
	}{
		{"file.pdf", "*.pdf", true},
		{"file.pdf", "*.doc", false},
		// Note: simpleGlobMatch is case-sensitive, matchGlob handles case conversion
		{"document", "doc*", true},
		{"document", "doc?ment", true},
		{"document", "doc??ment", false},
		{"abc", "*", true},
		{"abc", "???", true},
		{"abc", "????", false},
		{"test.tar.gz", "*.gz", true},
		{"test.tar.gz", "*.tar.gz", true},
	}

	for _, tt := range tests {
		t.Run(tt.str+"/"+tt.pattern, func(t *testing.T) {
			result := simpleGlobMatch(tt.str, tt.pattern)
			if result != tt.expected {
				t.Errorf("simpleGlobMatch(%q, %q) = %v, want %v", tt.str, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestMatchDomain(t *testing.T) {
	tests := []struct {
		host     string
		pattern  string
		expected bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "other.com", false},
		{"sub.example.com", "*.example.com", true},
		{"example.com", "*.example.com", true},
		{"deep.sub.example.com", "*.example.com", true},
		{"example.com", "*.other.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host+"/"+tt.pattern, func(t *testing.T) {
			result := matchDomain(tt.host, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchDomain(%q, %q) = %v, want %v", tt.host, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestSplitPatterns(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"*.pdf", []string{"*.pdf"}},
		{"*.pdf,*.doc", []string{"*.pdf", "*.doc"}},
		{"*.pdf, *.doc , *.txt", []string{"*.pdf", "*.doc", "*.txt"}},
		{",,,", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitPatterns(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitPatterns(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("splitPatterns(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}
