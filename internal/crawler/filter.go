package crawler

import (
	"net/url"
	"path"
	"strings"
)

// Filter determines which URLs should be crawled
type Filter struct {
	// Accept patterns (glob-style: *.pdf, *.zip)
	AcceptPatterns []string

	// Reject patterns
	RejectPatterns []string

	// Accept file extensions (without dot: pdf, zip)
	AcceptExtensions []string

	// Reject file extensions
	RejectExtensions []string

	// Only crawl same domain
	SameDomain bool

	// Allowed domains (if SameDomain is false)
	AllowedDomains []string

	// Excluded domains
	ExcludedDomains []string

	// Base URL for same-domain checking
	BaseURL *url.URL

	// Follow external links (different domain)
	FollowExternal bool

	// Include page requisites (images, css, js)
	PageRequisites bool
}

// NewFilter creates a new filter with default settings
func NewFilter(baseURL *url.URL) *Filter {
	return &Filter{
		BaseURL:        baseURL,
		SameDomain:     true,
		FollowExternal: false,
		PageRequisites: true,
	}
}

// ShouldCrawl returns true if the URL should be crawled
func (f *Filter) ShouldCrawl(u *url.URL, linkType LinkType) bool {
	// Always skip non-http(s) URLs
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	// Check domain restrictions
	if !f.isDomainAllowed(u) {
		// Exception: allow page requisites from any domain
		if f.PageRequisites && isPageRequisite(linkType) {
			// Allow but don't follow links from requisites
			return true
		}
		return false
	}

	// Get the file path/name
	filePath := u.Path
	fileName := path.Base(filePath)

	// Check reject patterns first (they take precedence)
	if f.matchesAny(fileName, f.RejectPatterns) {
		return false
	}

	// Check reject extensions
	ext := getExtension(fileName)
	if f.containsIgnoreCase(f.RejectExtensions, ext) {
		return false
	}

	// If accept patterns are specified, URL must match
	if len(f.AcceptPatterns) > 0 {
		if !f.matchesAny(fileName, f.AcceptPatterns) {
			return false
		}
	}

	// If accept extensions are specified, URL must match
	if len(f.AcceptExtensions) > 0 {
		if !f.containsIgnoreCase(f.AcceptExtensions, ext) {
			return false
		}
	}

	return true
}

// ShouldFollowLinks returns true if links from this URL should be followed
func (f *Filter) ShouldFollowLinks(u *url.URL, linkType LinkType) bool {
	// Don't follow links from non-HTML content
	if isPageRequisite(linkType) && linkType != LinkTypeAnchor {
		return false
	}

	// Don't follow external links unless configured
	if !f.isDomainAllowed(u) && !f.FollowExternal {
		return false
	}

	return true
}

// isDomainAllowed checks if the URL's domain is allowed
func (f *Filter) isDomainAllowed(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())

	// Check excluded domains first
	for _, excluded := range f.ExcludedDomains {
		if matchDomain(host, excluded) {
			return false
		}
	}

	// Check allowed domains if specified
	if len(f.AllowedDomains) > 0 {
		for _, allowed := range f.AllowedDomains {
			if matchDomain(host, allowed) {
				return true
			}
		}
		return false
	}

	// Check same domain
	if f.SameDomain && f.BaseURL != nil {
		baseHost := strings.ToLower(f.BaseURL.Hostname())
		return host == baseHost || strings.HasSuffix(host, "."+baseHost)
	}

	return true
}

// matchDomain checks if host matches pattern (supports wildcard *.example.com)
func matchDomain(host, pattern string) bool {
	pattern = strings.ToLower(pattern)
	host = strings.ToLower(host)

	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return host == pattern[2:] || strings.HasSuffix(host, suffix)
	}

	return host == pattern
}

// matchesAny checks if name matches any of the glob patterns
func (f *Filter) matchesAny(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlob(name, pattern) {
			return true
		}
	}
	return false
}

// matchGlob performs simple glob matching (supports * and ?)
func matchGlob(name, pattern string) bool {
	name = strings.ToLower(name)
	pattern = strings.ToLower(pattern)

	// Simple glob matching
	return simpleGlobMatch(name, pattern)
}

// simpleGlobMatch implements basic glob matching
func simpleGlobMatch(str, pattern string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Try matching rest of pattern with progressively shorter string
			pattern = pattern[1:]
			if len(pattern) == 0 {
				return true // Trailing * matches everything
			}
			for i := 0; i <= len(str); i++ {
				if simpleGlobMatch(str[i:], pattern) {
					return true
				}
			}
			return false

		case '?':
			if len(str) == 0 {
				return false
			}
			str = str[1:]
			pattern = pattern[1:]

		default:
			if len(str) == 0 || str[0] != pattern[0] {
				return false
			}
			str = str[1:]
			pattern = pattern[1:]
		}
	}

	return len(str) == 0
}

// getExtension returns the file extension without the dot
func getExtension(filename string) string {
	ext := path.Ext(filename)
	if len(ext) > 0 && ext[0] == '.' {
		return ext[1:]
	}
	return ext
}

// containsIgnoreCase checks if slice contains string (case-insensitive)
func (f *Filter) containsIgnoreCase(slice []string, s string) bool {
	s = strings.ToLower(s)
	for _, item := range slice {
		if strings.ToLower(item) == s {
			return true
		}
	}
	return false
}

// isPageRequisite checks if link type is a page requisite (needed to render page)
func isPageRequisite(linkType LinkType) bool {
	switch linkType {
	case LinkTypeImage, LinkTypeScript, LinkTypeStylesheet:
		return true
	default:
		return false
	}
}

// SetAcceptPatterns sets accept patterns from comma-separated string
func (f *Filter) SetAcceptPatterns(patterns string) {
	f.AcceptPatterns = splitPatterns(patterns)
}

// SetRejectPatterns sets reject patterns from comma-separated string
func (f *Filter) SetRejectPatterns(patterns string) {
	f.RejectPatterns = splitPatterns(patterns)
}

// SetAcceptExtensions sets accept extensions from comma-separated string
func (f *Filter) SetAcceptExtensions(extensions string) {
	f.AcceptExtensions = splitPatterns(extensions)
}

// SetRejectExtensions sets reject extensions from comma-separated string
func (f *Filter) SetRejectExtensions(extensions string) {
	f.RejectExtensions = splitPatterns(extensions)
}

// splitPatterns splits comma-separated patterns
func splitPatterns(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
