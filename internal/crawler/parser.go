package crawler

import (
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// LinkType represents the type of link found
type LinkType int

const (
	LinkTypeAnchor     LinkType = iota // <a href>
	LinkTypeImage                      // <img src>
	LinkTypeScript                     // <script src>
	LinkTypeStylesheet                 // <link rel="stylesheet" href>
	LinkTypeFrame                      // <frame src>, <iframe src>
	LinkTypeObject                     // <object data>, <embed src>
	LinkTypeForm                       // <form action>
	LinkTypeOther
)

// String returns string representation of link type
func (t LinkType) String() string {
	switch t {
	case LinkTypeAnchor:
		return "anchor"
	case LinkTypeImage:
		return "image"
	case LinkTypeScript:
		return "script"
	case LinkTypeStylesheet:
		return "stylesheet"
	case LinkTypeFrame:
		return "frame"
	case LinkTypeObject:
		return "object"
	case LinkTypeForm:
		return "form"
	default:
		return "other"
	}
}

// ExtractedLink represents a link found in HTML
type ExtractedLink struct {
	URL      string
	Type     LinkType
	Text     string // Anchor text for <a> tags
	Tag      string // HTML tag name
	Attr     string // Attribute name (href, src, etc.)
	BaseURL  *url.URL
	Resolved *url.URL // Resolved absolute URL
}

// ParseHTML extracts all links from HTML content
func ParseHTML(r io.Reader, baseURL *url.URL) ([]*ExtractedLink, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var links []*ExtractedLink
	var base = baseURL

	// First pass: look for <base href> tag
	var findBase func(*html.Node)
	findBase = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "base" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					if parsed, err := url.Parse(attr.Val); err == nil {
						base = baseURL.ResolveReference(parsed)
					}
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findBase(c)
		}
	}
	findBase(doc)

	// Second pass: extract all links
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if link := extractLink(n, base); link != nil {
				links = append(links, link)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(doc)

	return links, nil
}

// extractLink extracts link info from an HTML node
func extractLink(n *html.Node, base *url.URL) *ExtractedLink {
	tag := strings.ToLower(n.Data)

	var linkType LinkType
	var attrName string

	switch tag {
	case "a", "area":
		linkType = LinkTypeAnchor
		attrName = "href"
	case "img":
		linkType = LinkTypeImage
		attrName = "src"
	case "script":
		linkType = LinkTypeScript
		attrName = "src"
	case "link":
		// Check if it's a stylesheet
		isStylesheet := false
		for _, attr := range n.Attr {
			if attr.Key == "rel" && strings.Contains(strings.ToLower(attr.Val), "stylesheet") {
				isStylesheet = true
				break
			}
		}
		if isStylesheet {
			linkType = LinkTypeStylesheet
			attrName = "href"
		} else {
			return nil // Skip non-stylesheet links
		}
	case "frame", "iframe":
		linkType = LinkTypeFrame
		attrName = "src"
	case "object":
		linkType = LinkTypeObject
		attrName = "data"
	case "embed":
		linkType = LinkTypeObject
		attrName = "src"
	case "form":
		linkType = LinkTypeForm
		attrName = "action"
	case "video", "audio", "source":
		linkType = LinkTypeOther
		attrName = "src"
	default:
		return nil
	}

	// Find the URL attribute
	var href string
	for _, attr := range n.Attr {
		if attr.Key == attrName {
			href = strings.TrimSpace(attr.Val)
			break
		}
	}

	if href == "" {
		return nil
	}

	// Skip javascript:, mailto:, tel:, data: URLs
	lowerHref := strings.ToLower(href)
	if strings.HasPrefix(lowerHref, "javascript:") ||
		strings.HasPrefix(lowerHref, "mailto:") ||
		strings.HasPrefix(lowerHref, "tel:") ||
		strings.HasPrefix(lowerHref, "data:") ||
		strings.HasPrefix(lowerHref, "#") {
		return nil
	}

	// Parse and resolve URL
	parsed, err := url.Parse(href)
	if err != nil {
		return nil
	}

	resolved := base.ResolveReference(parsed)

	// Get anchor text for <a> tags
	var text string
	if tag == "a" {
		text = getTextContent(n)
	}

	return &ExtractedLink{
		URL:      href,
		Type:     linkType,
		Text:     text,
		Tag:      tag,
		Attr:     attrName,
		BaseURL:  base,
		Resolved: resolved,
	}
}

// getTextContent extracts text content from a node
func getTextContent(n *html.Node) string {
	var text strings.Builder

	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(n)

	return strings.TrimSpace(text.String())
}

// ParseCSS extracts URLs from CSS content
func ParseCSS(content string, baseURL *url.URL) []*ExtractedLink {
	var links []*ExtractedLink

	// Simple regex-like parsing for url(...) patterns
	// This is a simplified implementation
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\r", " ")

	i := 0
	for i < len(content) {
		// Find "url("
		idx := strings.Index(content[i:], "url(")
		if idx == -1 {
			break
		}
		i += idx + 4

		// Skip whitespace
		for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
			i++
		}

		if i >= len(content) {
			break
		}

		// Check for quotes
		quote := byte(0)
		if content[i] == '"' || content[i] == '\'' {
			quote = content[i]
			i++
		}

		// Extract URL
		start := i
		for i < len(content) {
			if quote != 0 && content[i] == quote {
				break
			}
			if quote == 0 && (content[i] == ')' || content[i] == ' ' || content[i] == '\t') {
				break
			}
			i++
		}

		if i > start {
			urlStr := strings.TrimSpace(content[start:i])

			// Skip data: URLs
			if !strings.HasPrefix(strings.ToLower(urlStr), "data:") {
				if parsed, err := url.Parse(urlStr); err == nil {
					resolved := baseURL.ResolveReference(parsed)
					links = append(links, &ExtractedLink{
						URL:      urlStr,
						Type:     LinkTypeStylesheet,
						Tag:      "css",
						Attr:     "url",
						BaseURL:  baseURL,
						Resolved: resolved,
					})
				}
			}
		}
	}

	return links
}

// IsHTMLContentType checks if content type indicates HTML
func IsHTMLContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml+xml")
}

// IsCSSContentType checks if content type indicates CSS
func IsCSSContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/css")
}
