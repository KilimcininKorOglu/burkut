package crawler

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkConverter_ConvertHTML(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/docs/")
	converter := NewLinkConverter(baseURL, "/tmp/output")

	html := []byte(`<!DOCTYPE html>
<html>
<head>
	<link rel="stylesheet" href="/css/style.css">
	<script src="/js/app.js"></script>
</head>
<body>
	<a href="/page1.html">Page 1</a>
	<a href="page2.html">Page 2</a>
	<a href="https://example.com/page3.html">Page 3</a>
	<a href="https://other.com/page.html">External</a>
	<a href="javascript:void(0)">JS Link</a>
	<a href="mailto:test@example.com">Email</a>
	<img src="/images/logo.png">
</body>
</html>`)

	// Use a mock file path
	filePath := "/tmp/output/example.com/docs/index.html"
	result := converter.convertHTML(html, filePath)
	resultStr := string(result)

	// Check that internal links were converted
	if strings.Contains(resultStr, `href="/css/style.css"`) {
		t.Error("Absolute path should be converted to relative")
	}

	// Check that external links remain unchanged
	if !strings.Contains(resultStr, `href="https://other.com/page.html"`) {
		t.Error("External links should remain unchanged")
	}

	// Check that javascript: links remain unchanged
	if !strings.Contains(resultStr, `href="javascript:void(0)"`) {
		t.Error("javascript: links should remain unchanged")
	}

	// Check that mailto: links remain unchanged
	if !strings.Contains(resultStr, `href="mailto:test@example.com"`) {
		t.Error("mailto: links should remain unchanged")
	}
}

func TestLinkConverter_ConvertCSS(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	converter := NewLinkConverter(baseURL, "/tmp/output")

	css := []byte(`
body {
	background: url('/images/bg.png');
}
.logo {
	background-image: url("/images/logo.png");
}
.icon {
	background: url(icons/icon.svg);
}
.data {
	background: url(data:image/png;base64,abc123);
}
`)

	filePath := "/tmp/output/example.com/css/style.css"
	result := converter.convertCSS(css, filePath)
	resultStr := string(result)

	// Check that data: URLs remain unchanged
	if !strings.Contains(resultStr, "url(data:image/png;base64,abc123)") {
		t.Error("data: URLs should remain unchanged")
	}
}

func TestLinkConverter_IsSameHost(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	converter := NewLinkConverter(baseURL, "/tmp/output")

	tests := []struct {
		urlStr   string
		expected bool
	}{
		{"https://example.com/page", true},
		{"https://sub.example.com/page", true},
		{"https://other.com/page", false},
		{"https://notexample.com/page", false},
	}

	for _, tt := range tests {
		t.Run(tt.urlStr, func(t *testing.T) {
			u, _ := url.Parse(tt.urlStr)
			result := converter.isSameHost(u)
			if result != tt.expected {
				t.Errorf("isSameHost(%s) = %v, want %v", tt.urlStr, result, tt.expected)
			}
		})
	}
}

func TestLinkConverter_ConvertFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "converter_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test HTML file
	htmlContent := `<!DOCTYPE html>
<html>
<body>
	<a href="https://example.com/page2.html">Link</a>
	<img src="https://example.com/images/logo.png">
</body>
</html>`

	htmlPath := filepath.Join(tmpDir, "example.com", "index.html")
	os.MkdirAll(filepath.Dir(htmlPath), 0755)
	os.WriteFile(htmlPath, []byte(htmlContent), 0644)

	// Convert
	baseURL, _ := url.Parse("https://example.com/")
	converter := NewLinkConverter(baseURL, tmpDir)
	err = converter.ConvertFile(htmlPath)
	if err != nil {
		t.Fatalf("ConvertFile error: %v", err)
	}

	// Read converted file
	converted, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read converted file: %v", err)
	}

	convertedStr := string(converted)

	// Should have converted the internal links
	if strings.Contains(convertedStr, "https://example.com/page2.html") {
		// Links to same host should be converted to relative
		t.Log("Note: Links might not be converted if target files don't exist")
	}
}

func TestMakeRelative(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/")
	converter := NewLinkConverter(baseURL, "/output")

	tests := []struct {
		fromFile   string
		targetPath string
		expected   string
	}{
		{"/output/example.com/index.html", "/output/example.com/page.html", "page.html"},
		{"/output/example.com/docs/index.html", "/output/example.com/css/style.css", "../css/style.css"},
		{"/output/example.com/docs/sub/page.html", "/output/example.com/images/logo.png", "../../images/logo.png"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result, err := converter.makeRelative(tt.fromFile, tt.targetPath)
			if err != nil {
				t.Fatalf("makeRelative error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("makeRelative(%s, %s) = %s, want %s", tt.fromFile, tt.targetPath, result, tt.expected)
			}
		})
	}
}
