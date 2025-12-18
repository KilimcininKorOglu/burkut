package crawler

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseHTML(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
	<link rel="stylesheet" href="/css/style.css">
	<script src="/js/app.js"></script>
</head>
<body>
	<a href="/page1">Page 1</a>
	<a href="https://example.com/page2">Page 2</a>
	<a href="page3.html">Page 3</a>
	<img src="/images/logo.png" alt="Logo">
	<a href="javascript:void(0)">JS Link</a>
	<a href="mailto:test@example.com">Email</a>
	<a href="#section">Anchor</a>
</body>
</html>`

	baseURL, _ := url.Parse("https://example.com/dir/")
	links, err := ParseHTML(strings.NewReader(html), baseURL)
	if err != nil {
		t.Fatalf("ParseHTML error: %v", err)
	}

	// Should find: stylesheet, script, 3 valid anchors, 1 image
	// Should skip: javascript:, mailto:, # anchor
	expectedCount := 6
	if len(links) != expectedCount {
		t.Errorf("Found %d links, want %d", len(links), expectedCount)
		for _, l := range links {
			t.Logf("  %s: %s -> %s", l.Type, l.URL, l.Resolved)
		}
	}

	// Check specific links
	foundStylesheet := false
	foundScript := false
	foundPage1 := false
	foundImage := false

	for _, link := range links {
		switch {
		case link.Type == LinkTypeStylesheet && link.URL == "/css/style.css":
			foundStylesheet = true
			if link.Resolved.String() != "https://example.com/css/style.css" {
				t.Errorf("Stylesheet resolved = %s, want https://example.com/css/style.css", link.Resolved)
			}

		case link.Type == LinkTypeScript && link.URL == "/js/app.js":
			foundScript = true

		case link.Type == LinkTypeAnchor && link.URL == "/page1":
			foundPage1 = true
			if link.Text != "Page 1" {
				t.Errorf("Anchor text = %q, want %q", link.Text, "Page 1")
			}

		case link.Type == LinkTypeImage && link.URL == "/images/logo.png":
			foundImage = true
		}
	}

	if !foundStylesheet {
		t.Error("Stylesheet link not found")
	}
	if !foundScript {
		t.Error("Script link not found")
	}
	if !foundPage1 {
		t.Error("Page 1 anchor not found")
	}
	if !foundImage {
		t.Error("Image link not found")
	}
}

func TestParseHTML_BaseTag(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
	<base href="https://other.com/base/">
</head>
<body>
	<a href="page.html">Link</a>
</body>
</html>`

	baseURL, _ := url.Parse("https://example.com/")
	links, err := ParseHTML(strings.NewReader(html), baseURL)
	if err != nil {
		t.Fatalf("ParseHTML error: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("Found %d links, want 1", len(links))
	}

	// Link should be resolved relative to <base href>
	expected := "https://other.com/base/page.html"
	if links[0].Resolved.String() != expected {
		t.Errorf("Resolved = %s, want %s", links[0].Resolved, expected)
	}
}

func TestParseHTML_RelativeURLs(t *testing.T) {
	tests := []struct {
		base     string
		href     string
		expected string
	}{
		{"https://example.com/dir/page.html", "other.html", "https://example.com/dir/other.html"},
		{"https://example.com/dir/page.html", "/absolute.html", "https://example.com/absolute.html"},
		{"https://example.com/dir/page.html", "../parent.html", "https://example.com/parent.html"},
		{"https://example.com/", "page.html", "https://example.com/page.html"},
		{"https://example.com/dir/", "sub/page.html", "https://example.com/dir/sub/page.html"},
	}

	for _, tt := range tests {
		t.Run(tt.href, func(t *testing.T) {
			html := `<html><body><a href="` + tt.href + `">Link</a></body></html>`
			baseURL, _ := url.Parse(tt.base)

			links, err := ParseHTML(strings.NewReader(html), baseURL)
			if err != nil {
				t.Fatalf("ParseHTML error: %v", err)
			}

			if len(links) != 1 {
				t.Fatalf("Found %d links, want 1", len(links))
			}

			if links[0].Resolved.String() != tt.expected {
				t.Errorf("Resolved = %s, want %s", links[0].Resolved, tt.expected)
			}
		})
	}
}

func TestParseCSS(t *testing.T) {
	css := `
body {
	background: url('/images/bg.png');
}
.logo {
	background-image: url("images/logo.png");
}
@font-face {
	src: url('/fonts/custom.woff2');
}
.icon {
	background: url(icons/icon.svg);
}
.data {
	background: url(data:image/png;base64,abc123);
}
`

	baseURL, _ := url.Parse("https://example.com/css/")
	links := ParseCSS(css, baseURL)

	// Should find 4 URLs (skip data: URL)
	if len(links) != 4 {
		t.Errorf("Found %d links, want 4", len(links))
		for _, l := range links {
			t.Logf("  %s", l.URL)
		}
	}

	// Check one resolved URL
	found := false
	for _, link := range links {
		if link.URL == "/images/bg.png" {
			found = true
			expected := "https://example.com/images/bg.png"
			if link.Resolved.String() != expected {
				t.Errorf("Resolved = %s, want %s", link.Resolved, expected)
			}
		}
	}
	if !found {
		t.Error("/images/bg.png not found")
	}
}

func TestLinkType_String(t *testing.T) {
	tests := []struct {
		lt       LinkType
		expected string
	}{
		{LinkTypeAnchor, "anchor"},
		{LinkTypeImage, "image"},
		{LinkTypeScript, "script"},
		{LinkTypeStylesheet, "stylesheet"},
		{LinkTypeFrame, "frame"},
	}

	for _, tt := range tests {
		if tt.lt.String() != tt.expected {
			t.Errorf("%v.String() = %s, want %s", tt.lt, tt.lt.String(), tt.expected)
		}
	}
}

func TestIsHTMLContentType(t *testing.T) {
	tests := []struct {
		ct       string
		expected bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"TEXT/HTML", true},
		{"application/xhtml+xml", true},
		{"text/plain", false},
		{"application/json", false},
	}

	for _, tt := range tests {
		if IsHTMLContentType(tt.ct) != tt.expected {
			t.Errorf("IsHTMLContentType(%q) = %v, want %v", tt.ct, !tt.expected, tt.expected)
		}
	}
}

func TestIsCSSContentType(t *testing.T) {
	tests := []struct {
		ct       string
		expected bool
	}{
		{"text/css", true},
		{"text/css; charset=utf-8", true},
		{"TEXT/CSS", true},
		{"text/html", false},
	}

	for _, tt := range tests {
		if IsCSSContentType(tt.ct) != tt.expected {
			t.Errorf("IsCSSContentType(%q) = %v, want %v", tt.ct, !tt.expected, tt.expected)
		}
	}
}
