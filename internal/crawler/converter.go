package crawler

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LinkConverter converts links in HTML/CSS files to local paths
type LinkConverter struct {
	baseURL   *url.URL
	outputDir string
}

// NewLinkConverter creates a new link converter
func NewLinkConverter(baseURL *url.URL, outputDir string) *LinkConverter {
	return &LinkConverter{
		baseURL:   baseURL,
		outputDir: outputDir,
	}
}

// ConvertFile converts links in a file to local paths
func (c *LinkConverter) ConvertFile(filePath string) error {
	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Determine file type
	ext := strings.ToLower(filepath.Ext(filePath))
	var converted []byte

	switch ext {
	case ".html", ".htm", ".xhtml":
		converted = c.convertHTML(content, filePath)
	case ".css":
		converted = c.convertCSS(content, filePath)
	default:
		return nil // Skip non-HTML/CSS files
	}

	// Write converted content
	return os.WriteFile(filePath, converted, 0644)
}

// convertHTML converts links in HTML content
func (c *LinkConverter) convertHTML(content []byte, filePath string) []byte {
	// Patterns for HTML attributes that contain URLs
	patterns := []struct {
		regex *regexp.Regexp
		attr  string
	}{
		{regexp.MustCompile(`(?i)(href\s*=\s*["'])([^"']+)(["'])`), "href"},
		{regexp.MustCompile(`(?i)(src\s*=\s*["'])([^"']+)(["'])`), "src"},
		{regexp.MustCompile(`(?i)(action\s*=\s*["'])([^"']+)(["'])`), "action"},
		{regexp.MustCompile(`(?i)(data\s*=\s*["'])([^"']+)(["'])`), "data"},
		{regexp.MustCompile(`(?i)(poster\s*=\s*["'])([^"']+)(["'])`), "poster"},
	}

	result := content
	for _, p := range patterns {
		result = p.regex.ReplaceAllFunc(result, func(match []byte) []byte {
			parts := p.regex.FindSubmatch(match)
			if len(parts) < 4 {
				return match
			}

			prefix := parts[1]
			urlStr := string(parts[2])
			suffix := parts[3]

			// Convert URL to local path
			localPath := c.urlToLocalPath(urlStr, filePath)
			if localPath == "" {
				return match // Keep original
			}

			return append(append(prefix, []byte(localPath)...), suffix...)
		})
	}

	// Also convert url() in inline styles
	styleRegex := regexp.MustCompile(`(?i)(style\s*=\s*["'])([^"']*)(["'])`)
	result = styleRegex.ReplaceAllFunc(result, func(match []byte) []byte {
		parts := styleRegex.FindSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		prefix := parts[1]
		styleContent := parts[2]
		suffix := parts[3]

		converted := c.convertCSSUrls(styleContent, filePath)
		return append(append(prefix, converted...), suffix...)
	})

	return result
}

// convertCSS converts URLs in CSS content
func (c *LinkConverter) convertCSS(content []byte, filePath string) []byte {
	return c.convertCSSUrls(content, filePath)
}

// convertCSSUrls converts url(...) patterns in CSS
func (c *LinkConverter) convertCSSUrls(content []byte, filePath string) []byte {
	// Pattern for url(...) in CSS
	urlRegex := regexp.MustCompile(`(?i)(url\s*\(\s*)(["']?)([^)"']+)(["']?\s*\))`)

	return urlRegex.ReplaceAllFunc(content, func(match []byte) []byte {
		parts := urlRegex.FindSubmatch(match)
		if len(parts) < 5 {
			return match
		}

		prefix := parts[1]
		quote := parts[2]
		urlStr := string(parts[3])
		suffix := parts[4]

		// Skip data URLs
		if strings.HasPrefix(strings.ToLower(urlStr), "data:") {
			return match
		}

		// Convert URL to local path
		localPath := c.urlToLocalPath(urlStr, filePath)
		if localPath == "" {
			return match
		}

		var buf bytes.Buffer
		buf.Write(prefix)
		buf.Write(quote)
		buf.WriteString(localPath)
		buf.Write(suffix)
		return buf.Bytes()
	})
}

// urlToLocalPath converts an absolute or relative URL to a local file path
func (c *LinkConverter) urlToLocalPath(urlStr string, currentFile string) string {
	// Skip javascript:, mailto:, tel:, data:, # links
	lower := strings.ToLower(urlStr)
	if strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(urlStr, "#") {
		return ""
	}

	// Parse the URL
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	// Resolve relative URLs
	resolved := c.baseURL.ResolveReference(parsed)

	// Check if it's on the same host (or subdomain)
	if !c.isSameHost(resolved) {
		return "" // Keep external links as-is
	}

	// Convert to local path
	localPath := c.urlPathToLocal(resolved)

	// Make path relative to current file
	relPath, err := c.makeRelative(currentFile, localPath)
	if err != nil {
		return localPath
	}

	return relPath
}

// isSameHost checks if URL is on the same host (including subdomains)
func (c *LinkConverter) isSameHost(u *url.URL) bool {
	baseHost := strings.ToLower(c.baseURL.Hostname())
	urlHost := strings.ToLower(u.Hostname())

	return urlHost == baseHost || strings.HasSuffix(urlHost, "."+baseHost)
}

// urlPathToLocal converts a URL path to a local file path
func (c *LinkConverter) urlPathToLocal(u *url.URL) string {
	// Start with host directory
	path := u.Hostname()

	// Add URL path
	urlPath := u.Path
	if urlPath == "" || urlPath == "/" {
		urlPath = "/index.html"
	}

	// Handle directory paths
	if strings.HasSuffix(urlPath, "/") {
		urlPath += "index.html"
	}

	// Add extension if missing
	if filepath.Ext(urlPath) == "" {
		urlPath += ".html"
	}

	path = filepath.Join(path, urlPath)

	return filepath.Join(c.outputDir, filepath.Clean(path))
}

// makeRelative makes targetPath relative to fromFile
func (c *LinkConverter) makeRelative(fromFile, targetPath string) (string, error) {
	fromDir := filepath.Dir(fromFile)

	rel, err := filepath.Rel(fromDir, targetPath)
	if err != nil {
		return "", err
	}

	// Use forward slashes for URLs
	return filepath.ToSlash(rel), nil
}

// ConvertDirectory converts all HTML/CSS files in a directory
func (c *LinkConverter) ConvertDirectory(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".html" || ext == ".htm" || ext == ".xhtml" || ext == ".css" {
			if err := c.ConvertFile(path); err != nil {
				return err
			}
		}

		return nil
	})
}

// ConvertAllFiles converts links in all downloaded files
func ConvertAllFiles(queue *URLQueue, outputDir string, baseURL *url.URL) error {
	converter := NewLinkConverter(baseURL, outputDir)

	for _, item := range queue.Items() {
		if item.Status != StatusCompleted || item.LocalPath == "" {
			continue
		}

		if err := converter.ConvertFile(item.LocalPath); err != nil {
			// Log but continue
			continue
		}
	}

	return nil
}
