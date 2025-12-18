package crawler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RobotsRule represents a single allow/disallow rule
type RobotsRule struct {
	Path    string
	Allowed bool
}

// RobotsData represents parsed robots.txt data for a site
type RobotsData struct {
	Rules      []RobotsRule
	CrawlDelay time.Duration
	Sitemaps   []string
}

// RobotsChecker manages robots.txt checking for multiple domains
type RobotsChecker struct {
	cache      map[string]*RobotsData
	mu         sync.RWMutex
	userAgent  string
	httpClient *http.Client
	enabled    bool
}

// NewRobotsChecker creates a new robots.txt checker
func NewRobotsChecker(userAgent string, enabled bool) *RobotsChecker {
	return &RobotsChecker{
		cache:     make(map[string]*RobotsData),
		userAgent: userAgent,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		enabled: enabled,
	}
}

// IsAllowed checks if a URL is allowed by robots.txt
func (r *RobotsChecker) IsAllowed(ctx context.Context, u *url.URL) (bool, error) {
	if !r.enabled {
		return true, nil
	}

	// Get robots data for this host
	data, err := r.getRobotsData(ctx, u)
	if err != nil {
		// If we can't fetch robots.txt, assume allowed
		return true, nil
	}

	if data == nil {
		return true, nil
	}

	return r.checkRules(u.Path, data.Rules), nil
}

// GetCrawlDelay returns the crawl delay for a host
func (r *RobotsChecker) GetCrawlDelay(ctx context.Context, u *url.URL) time.Duration {
	if !r.enabled {
		return 0
	}

	data, err := r.getRobotsData(ctx, u)
	if err != nil || data == nil {
		return 0
	}

	return data.CrawlDelay
}

// getRobotsData fetches and parses robots.txt for a host
func (r *RobotsChecker) getRobotsData(ctx context.Context, u *url.URL) (*RobotsData, error) {
	host := u.Scheme + "://" + u.Host

	// Check cache first
	r.mu.RLock()
	data, exists := r.cache[host]
	r.mu.RUnlock()

	if exists {
		return data, nil
	}

	// Fetch robots.txt
	robotsURL := host + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", r.userAgent)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		// Cache empty result to avoid repeated failures
		r.mu.Lock()
		r.cache[host] = nil
		r.mu.Unlock()
		return nil, err
	}
	defer resp.Body.Close()

	// If not found or error, cache empty result
	if resp.StatusCode != http.StatusOK {
		r.mu.Lock()
		r.cache[host] = nil
		r.mu.Unlock()
		return nil, nil
	}

	// Parse robots.txt
	data = parseRobotsTxt(resp.Body, r.userAgent)

	// Cache result
	r.mu.Lock()
	r.cache[host] = data
	r.mu.Unlock()

	return data, nil
}

// checkRules checks if a path is allowed by the rules
func (r *RobotsChecker) checkRules(path string, rules []RobotsRule) bool {
	// Find the most specific matching rule
	var matchedRule *RobotsRule
	matchedLen := -1

	for i := range rules {
		rule := &rules[i]
		if matchesRobotsPath(path, rule.Path) {
			if len(rule.Path) > matchedLen {
				matchedRule = rule
				matchedLen = len(rule.Path)
			}
		}
	}

	// No matching rule means allowed
	if matchedRule == nil {
		return true
	}

	return matchedRule.Allowed
}

// parseRobotsTxt parses robots.txt content
func parseRobotsTxt(r io.Reader, targetUserAgent string) *RobotsData {
	data := &RobotsData{
		Rules:    make([]RobotsRule, 0),
		Sitemaps: make([]string, 0),
	}

	scanner := bufio.NewScanner(r)

	var currentUserAgents []string
	var currentRules []RobotsRule
	var currentCrawlDelay time.Duration

	flushGroup := func() {
		// Check if this group applies to us
		applies := false
		for _, ua := range currentUserAgents {
			if ua == "*" || strings.Contains(strings.ToLower(targetUserAgent), strings.ToLower(ua)) {
				applies = true
				break
			}
		}

		if applies {
			data.Rules = append(data.Rules, currentRules...)
			if currentCrawlDelay > data.CrawlDelay {
				data.CrawlDelay = currentCrawlDelay
			}
		}

		currentUserAgents = nil
		currentRules = nil
		currentCrawlDelay = 0
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split into key:value
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "user-agent":
			// If we already have rules, flush them
			if len(currentRules) > 0 {
				flushGroup()
			}
			currentUserAgents = append(currentUserAgents, value)

		case "disallow":
			if value != "" {
				currentRules = append(currentRules, RobotsRule{
					Path:    value,
					Allowed: false,
				})
			}

		case "allow":
			if value != "" {
				currentRules = append(currentRules, RobotsRule{
					Path:    value,
					Allowed: true,
				})
			}

		case "crawl-delay":
			if delay, err := parseDelay(value); err == nil {
				currentCrawlDelay = delay
			}

		case "sitemap":
			data.Sitemaps = append(data.Sitemaps, value)
		}
	}

	// Flush final group
	if len(currentUserAgents) > 0 {
		flushGroup()
	}

	return data
}

// parseDelay parses crawl delay value
func parseDelay(s string) (time.Duration, error) {
	// Try as integer seconds first
	var seconds float64
	_, err := fmt.Sscanf(s, "%f", &seconds)
	if err != nil {
		return 0, err
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// matchesRobotsPath checks if a URL path matches a robots.txt rule path
func matchesRobotsPath(urlPath, rulePath string) bool {
	// Handle wildcards
	if strings.Contains(rulePath, "*") {
		return matchRobotsGlob(urlPath, rulePath)
	}

	// Handle $ (end anchor)
	if strings.HasSuffix(rulePath, "$") {
		return urlPath == rulePath[:len(rulePath)-1]
	}

	// Simple prefix match
	return strings.HasPrefix(urlPath, rulePath)
}

// matchRobotsGlob matches URL path against robots.txt glob pattern
func matchRobotsGlob(urlPath, pattern string) bool {
	// Split pattern by *
	parts := strings.Split(pattern, "*")

	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}

		// Handle $ at the end
		if i == len(parts)-1 && strings.HasSuffix(part, "$") {
			part = part[:len(part)-1]
			return strings.HasSuffix(urlPath, part)
		}

		idx := strings.Index(urlPath[pos:], part)
		if idx == -1 {
			return false
		}

		// First part must match at start (unless pattern starts with *)
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}

		pos += idx + len(part)
	}

	return true
}
