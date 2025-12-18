// Burkut - Modern Download Manager
// Named after the golden eagle (berkut) in Turkish mythology
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/config"
	"github.com/kilimcininkoroglu/burkut/internal/crawler"
	"github.com/kilimcininkoroglu/burkut/internal/download"
	"github.com/kilimcininkoroglu/burkut/internal/engine"
	"github.com/kilimcininkoroglu/burkut/internal/hooks"
	"github.com/kilimcininkoroglu/burkut/internal/metalink"
	"github.com/kilimcininkoroglu/burkut/internal/metrics"
	"github.com/kilimcininkoroglu/burkut/internal/protocol"
	btorrent "github.com/kilimcininkoroglu/burkut/internal/torrent"
	"github.com/kilimcininkoroglu/burkut/internal/tui"
	"github.com/kilimcininkoroglu/burkut/internal/ui"
	"github.com/kilimcininkoroglu/burkut/internal/version"
)

// Exit codes
const (
	ExitSuccess        = 0
	ExitGeneralError   = 1
	ExitParseError     = 2
	ExitNetworkError   = 3
	ExitAuthError      = 4
	ExitTLSError       = 5
	ExitChecksumError  = 6
	ExitTimeoutError   = 7
	ExitInterrupted    = 8
)

// headerList is a custom flag type for multiple headers
type headerList []string

func (h *headerList) String() string {
	return strings.Join(*h, ", ")
}

func (h *headerList) Set(value string) error {
	*h = append(*h, value)
	return nil
}

// CLIConfig holds CLI configuration
type CLIConfig struct {
	Output      string
	OutputDir   string
	Continue    bool
	Connections int
	Quiet       bool
	Verbose     bool
	NoColor     bool
	Progress    string // "bar", "minimal", "json", "none"
	Timeout     time.Duration
	ShowVersion bool
	ShowHelp    bool
	// Phase 2 features
	LimitRate    string // e.g., "10M", "500K"
	Checksum     string // e.g., "sha256:abc123..."
	AutoVerify   bool   // Auto-detect and verify checksum from .sha256 files
	ConfigFile   string // custom config file path
	Profile      string // named profile to use
	InitConfig   bool   // generate default config
	Proxy        string // HTTP/HTTPS proxy URL
	NoCheckCert  bool   // Skip TLS certificate verification
	// Phase 1.0 features
	InputFile     string // File containing URLs to download
	OnComplete    string // Command to run on completion
	OnError       string // Command to run on error
	WebhookURL    string // Webhook URL for notifications
	MirrorURLs    string // Comma-separated mirror URLs
	UseHTTP3      bool   // Use HTTP/3 (QUIC) protocol
	ForceHTTP1    bool   // Force HTTP/1.1 (disable HTTP/2)
	ForceHTTP2    bool   // Force HTTP/2 (fail if not supported)
	// Conditional download
	Timestamping bool // Only download if remote is newer
	// Security
	PinnedPubKey string // SHA256 public key pin for certificate pinning
	// Authentication
	UseNetrc    bool       // Use .netrc for authentication
	Headers     headerList // Custom headers
	BasicAuth   string     // Basic auth (user:password)
	// TUI
	UseTUI bool // Use interactive TUI mode
	// Recursive download (spider mode)
	Recursive       bool   // Enable recursive download
	MaxDepth        int    // Maximum recursion depth (0 = start URL only)
	SpanHosts       bool   // Follow links to other hosts
	AcceptPatterns  string // Accept patterns (comma-separated)
	RejectPatterns  string // Reject patterns (comma-separated)
	AcceptExt       string // Accept extensions (comma-separated)
	RejectExt       string // Reject extensions (comma-separated)
	WaitTime        string // Wait time between requests (e.g., "1s")
	RandomWait      bool   // Add random 0-500ms to wait time
	RobotsOff       bool   // Ignore robots.txt
	PageRequisites  bool   // Download page requisites (CSS, JS, images)
	ConvertLinks    bool   // Convert links to local paths
	MirrorMode      bool   // Mirror mode (-m = -r -N -l inf)
	SpiderMode      bool   // Spider mode (list URLs only, no download)
	// Metrics
	MetricsAddr     string // Prometheus metrics endpoint address (e.g., ":9090")
}

func main() {
	cliConfig := parseFlags()

	if cliConfig.ShowVersion {
		fmt.Println(version.Full())
		os.Exit(ExitSuccess)
	}

	// Start metrics server if requested
	if cliConfig.MetricsAddr != "" {
		m := metrics.New()
		metricsServer := metrics.NewServer(cliConfig.MetricsAddr, m)
		if err := metricsServer.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to start metrics server: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Metrics server listening on %s\n", cliConfig.MetricsAddr)
			defer metricsServer.Stop()
		}
	}

	// Handle --init-config
	if cliConfig.InitConfig {
		exitCode := initConfig()
		os.Exit(exitCode)
	}

	// Check for batch download mode first
	if cliConfig.InputFile != "" {
		exitCode := runBatchDownload(cliConfig)
		os.Exit(exitCode)
	}

	if cliConfig.ShowHelp || flag.NArg() == 0 {
		printUsage()
		if flag.NArg() == 0 && !cliConfig.ShowHelp {
			os.Exit(ExitParseError)
		}
		os.Exit(ExitSuccess)
	}

	// Get URL from arguments
	urlArg := flag.Arg(0)
	if urlArg == "" {
		fmt.Fprintln(os.Stderr, "Error: URL is required")
		printUsage()
		os.Exit(ExitParseError)
	}

	// Check if argument is a torrent file or magnet link
	if btorrent.IsTorrentFile(urlArg) || btorrent.IsMagnetURI(urlArg) {
		exitCode := runTorrentDownload(cliConfig, urlArg)
		os.Exit(exitCode)
	}

	// Check if argument is a metalink file
	if metalink.IsMetalink(urlArg) {
		exitCode := runMetalinkDownload(cliConfig, urlArg)
		os.Exit(exitCode)
	}

	// Handle mirror mode shortcuts
	if cliConfig.MirrorMode {
		cliConfig.Recursive = true
		cliConfig.Timestamping = true
		cliConfig.MaxDepth = 0 // Unlimited
		cliConfig.PageRequisites = true
	}

	// Run download
	var exitCode int
	if cliConfig.SpiderMode {
		exitCode = runSpiderMode(cliConfig, urlArg)
	} else if cliConfig.Recursive {
		exitCode = runRecursiveDownload(cliConfig, urlArg)
	} else if isFTPURL(urlArg) {
		exitCode = runFTPDownload(cliConfig, urlArg)
	} else if cliConfig.UseTUI {
		exitCode = runDownloadTUI(cliConfig, urlArg)
	} else {
		exitCode = runDownload(cliConfig, urlArg)
	}
	os.Exit(exitCode)
}

func parseFlags() CLIConfig {
	cfg := CLIConfig{}

	// Basic options
	flag.StringVar(&cfg.Output, "o", "", "Output filename")
	flag.StringVar(&cfg.Output, "output", "", "Output filename")
	flag.StringVar(&cfg.OutputDir, "P", ".", "Output directory")
	flag.StringVar(&cfg.OutputDir, "output-dir", ".", "Output directory")
	flag.BoolVar(&cfg.Continue, "c", false, "Continue/resume download")
	flag.BoolVar(&cfg.Continue, "continue", false, "Continue/resume download")
	flag.IntVar(&cfg.Connections, "n", 4, "Number of parallel connections")
	flag.IntVar(&cfg.Connections, "connections", 4, "Number of parallel connections")
	flag.BoolVar(&cfg.Quiet, "q", false, "Quiet mode (no progress)")
	flag.BoolVar(&cfg.Quiet, "quiet", false, "Quiet mode (no progress)")
	flag.BoolVar(&cfg.Verbose, "v", false, "Verbose output")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Verbose output")
	flag.BoolVar(&cfg.NoColor, "no-color", false, "Disable colored output")
	flag.StringVar(&cfg.Progress, "progress", "bar", "Progress style: bar, minimal, json, none")
	flag.DurationVar(&cfg.Timeout, "T", 30*time.Second, "Connection timeout")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "Connection timeout")
	flag.BoolVar(&cfg.ShowVersion, "V", false, "Show version")
	flag.BoolVar(&cfg.ShowVersion, "version", false, "Show version")
	flag.BoolVar(&cfg.ShowHelp, "h", false, "Show help")
	flag.BoolVar(&cfg.ShowHelp, "help", false, "Show help")

	// Phase 2 options
	flag.StringVar(&cfg.LimitRate, "limit-rate", "", "Limit download speed (e.g., 10M, 500K)")
	flag.StringVar(&cfg.Checksum, "checksum", "", "Verify checksum (e.g., sha256:abc123...)")
	flag.BoolVar(&cfg.AutoVerify, "verify", false, "Auto-detect and verify checksum (.sha256, .md5, etc.)")
	flag.StringVar(&cfg.ConfigFile, "config", "", "Use custom config file")
	flag.StringVar(&cfg.Profile, "profile", "", "Use named profile from config")
	flag.BoolVar(&cfg.InitConfig, "init-config", false, "Generate default config file")
	flag.StringVar(&cfg.Proxy, "proxy", "", "Proxy URL (http://host:port or socks5://host:port)")
	flag.BoolVar(&cfg.NoCheckCert, "no-check-certificate", false, "Skip TLS certificate verification")

	// Phase 1.0 options
	flag.StringVar(&cfg.InputFile, "i", "", "Read URLs from file (one per line)")
	flag.StringVar(&cfg.InputFile, "input-file", "", "Read URLs from file (one per line)")
	flag.StringVar(&cfg.OnComplete, "on-complete", "", "Command to run after successful download")
	flag.StringVar(&cfg.OnError, "on-error", "", "Command to run after failed download")
	flag.StringVar(&cfg.WebhookURL, "webhook", "", "Webhook URL for download notifications")
	flag.StringVar(&cfg.MirrorURLs, "mirrors", "", "Comma-separated mirror URLs for fallback")
	flag.BoolVar(&cfg.UseHTTP3, "http3", false, "Use HTTP/3 (QUIC) protocol (experimental)")
	flag.BoolVar(&cfg.ForceHTTP1, "http1", false, "Force HTTP/1.1 (disable HTTP/2)")
	flag.BoolVar(&cfg.ForceHTTP2, "http2", false, "Force HTTP/2 (fail if server doesn't support)")

	// Conditional download options
	flag.BoolVar(&cfg.Timestamping, "N", false, "Only download if remote file is newer than local")
	flag.BoolVar(&cfg.Timestamping, "timestamping", false, "Only download if remote file is newer than local")

	// Security options
	flag.StringVar(&cfg.PinnedPubKey, "pinnedpubkey", "", "SHA256 public key pin (sha256//base64hash)")

	// Authentication options
	flag.BoolVar(&cfg.UseNetrc, "netrc", false, "Use ~/.netrc for authentication")
	flag.Var(&cfg.Headers, "H", "Add custom header (can be used multiple times)")
	flag.Var(&cfg.Headers, "header", "Add custom header (can be used multiple times)")
	flag.StringVar(&cfg.BasicAuth, "u", "", "Basic auth credentials (user:password)")
	flag.StringVar(&cfg.BasicAuth, "user", "", "Basic auth credentials (user:password)")

	// TUI option
	flag.BoolVar(&cfg.UseTUI, "tui", false, "Use interactive TUI mode")

	// Recursive download options
	flag.BoolVar(&cfg.Recursive, "r", false, "Enable recursive download")
	flag.BoolVar(&cfg.Recursive, "recursive", false, "Enable recursive download")
	flag.IntVar(&cfg.MaxDepth, "l", 5, "Maximum recursion depth (0 = start URL only)")
	flag.IntVar(&cfg.MaxDepth, "level", 5, "Maximum recursion depth (0 = start URL only)")
	flag.BoolVar(&cfg.SpanHosts, "span-hosts", false, "Follow links to other hosts")
	flag.StringVar(&cfg.AcceptPatterns, "A", "", "Accept patterns (comma-separated, e.g., '*.pdf,*.doc')")
	flag.StringVar(&cfg.AcceptPatterns, "accept", "", "Accept patterns (comma-separated)")
	flag.StringVar(&cfg.RejectPatterns, "R", "", "Reject patterns (comma-separated, e.g., '*.exe,*.zip')")
	flag.StringVar(&cfg.RejectPatterns, "reject", "", "Reject patterns (comma-separated)")
	flag.StringVar(&cfg.AcceptExt, "accept-ext", "", "Accept extensions (comma-separated, e.g., 'html,htm,php')")
	flag.StringVar(&cfg.RejectExt, "reject-ext", "", "Reject extensions (comma-separated)")
	flag.StringVar(&cfg.WaitTime, "w", "", "Wait time between requests (e.g., '1s', '500ms')")
	flag.StringVar(&cfg.WaitTime, "wait", "", "Wait time between requests")
	flag.BoolVar(&cfg.RandomWait, "random-wait", false, "Add random 0-500ms to wait time")
	flag.BoolVar(&cfg.RobotsOff, "e", false, "Ignore robots.txt")
	flag.BoolVar(&cfg.RobotsOff, "robots-off", false, "Ignore robots.txt")
	flag.BoolVar(&cfg.PageRequisites, "p", false, "Download page requisites (CSS, JS, images)")
	flag.BoolVar(&cfg.PageRequisites, "page-requisites", false, "Download page requisites")
	flag.BoolVar(&cfg.ConvertLinks, "k", false, "Convert links to local paths")
	flag.BoolVar(&cfg.ConvertLinks, "convert-links", false, "Convert links to local paths")
	flag.BoolVar(&cfg.MirrorMode, "m", false, "Mirror mode (-r -N -l inf --page-requisites)")
	flag.BoolVar(&cfg.MirrorMode, "mirror", false, "Mirror mode")
	flag.BoolVar(&cfg.SpiderMode, "spider", false, "Spider mode (list URLs only, no download)")

	// Metrics options
	flag.StringVar(&cfg.MetricsAddr, "metrics-addr", "", "Prometheus metrics endpoint (e.g., :9090)")

	flag.Usage = printUsage
	flag.Parse()

	// Apply quiet mode
	if cfg.Quiet {
		cfg.Progress = "none"
	}

	return cfg
}

func runDownload(cliCfg CLIConfig, url string) int {
	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nInterrupted, saving state...")
		cancel()
	}()

	// Load config file (if exists)
	cfg, err := loadConfig(cliCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return ExitParseError
	}

	// Parse rate limit
	var rateLimiter *engine.RateLimiter
	if cliCfg.LimitRate != "" {
		bytesPerSec, err := config.ParseBandwidth(cliCfg.LimitRate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid rate limit: %v\n", err)
			return ExitParseError
		}
		rateLimiter = engine.NewRateLimiter(bytesPerSec)
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Rate limit: %s/s\n", ui.FormatBytes(bytesPerSec))
		}
	} else if cfg.Bandwidth.GlobalLimit != "" {
		bytesPerSec, err := config.ParseBandwidth(cfg.Bandwidth.GlobalLimit)
		if err == nil && bytesPerSec > 0 {
			rateLimiter = engine.NewRateLimiter(bytesPerSec)
		}
	}

	// Parse expected checksum
	var expectedChecksum *engine.Checksum
	if cliCfg.Checksum != "" {
		expectedChecksum, err = engine.ParseChecksumAuto(cliCfg.Checksum)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid checksum: %v\n", err)
			return ExitParseError
		}
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Expected checksum: %s\n", expectedChecksum.String())
		}
	} else if cliCfg.AutoVerify {
		// Auto-verify will be handled after download
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Auto-verify enabled\n")
		}
	}


	// Build HTTP client options
	httpOpts := []protocol.HTTPClientOption{
		protocol.WithTimeout(cliCfg.Timeout),
		protocol.WithUserAgent(fmt.Sprintf("Burkut/%s", version.Version)),
	}

	// Add proxy if specified (CLI takes precedence over config)
	proxyURL := cliCfg.Proxy
	if proxyURL == "" && cfg.Proxy.HTTP != "" {
		proxyURL = cfg.Proxy.HTTP
	}
	if proxyURL != "" {
		if strings.HasPrefix(proxyURL, "socks5://") {
			httpOpts = append(httpOpts, protocol.WithSOCKS5Proxy(proxyURL, nil))
		} else {
			httpOpts = append(httpOpts, protocol.WithProxy(proxyURL))
		}
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Using proxy: %s\n", proxyURL)
		}
	}

	// Skip certificate verification if requested
	if cliCfg.NoCheckCert || !cfg.TLS.Verify {
		httpOpts = append(httpOpts, protocol.WithInsecureSkipVerify(true))
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Warning: TLS certificate verification disabled\n")
		}
	}

	// HTTP version control
	if cliCfg.ForceHTTP1 {
		httpOpts = append(httpOpts, protocol.WithForceHTTP1(true))
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Forcing HTTP/1.1\n")
		}
	} else if cliCfg.ForceHTTP2 {
		httpOpts = append(httpOpts, protocol.WithForceHTTP2(true))
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Forcing HTTP/2\n")
		}
	}

	// Certificate pinning
	if cliCfg.PinnedPubKey != "" {
		httpOpts = append(httpOpts, protocol.WithPinnedPublicKey(cliCfg.PinnedPubKey))
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Certificate pinning enabled\n")
		}
	}

	// Handle authentication
	authUser, authPass := "", ""

	// 1. Try netrc first
	if cliCfg.UseNetrc {
		netrc, err := config.LoadNetrc()
		if err != nil {
			if cliCfg.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: Could not load netrc: %v\n", err)
			}
		} else if login, password, found := netrc.GetCredentials(url); found {
			authUser, authPass = login, password
			if cliCfg.Verbose {
				fmt.Fprintf(os.Stderr, "Using credentials from netrc for %s\n", url)
			}
		}
	}

	// 2. CLI -u/--user overrides netrc
	if cliCfg.BasicAuth != "" {
		parts := strings.SplitN(cliCfg.BasicAuth, ":", 2)
		authUser = parts[0]
		if len(parts) > 1 {
			authPass = parts[1]
		}
	}

	// Apply authentication
	if authUser != "" {
		httpOpts = append(httpOpts, protocol.WithBasicAuth(authUser, authPass))
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Using Basic auth for user: %s\n", authUser)
		}
	}

	// Add custom headers
	if len(cliCfg.Headers) > 0 {
		headers := make(map[string]string)
		for _, h := range cliCfg.Headers {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) == 2 {
				headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		httpOpts = append(httpOpts, protocol.WithHeaders(headers))
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Custom headers: %d\n", len(headers))
		}
	}

	// Create HTTP client
	httpClient := protocol.NewHTTPClient(httpOpts...)

	// HTTP/3 client (optional)
	var http3Client *protocol.HTTP3Client
	if cliCfg.UseHTTP3 {
		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Using HTTP/3 (QUIC) protocol\n")
		}
		http3Client = protocol.NewHTTP3Client(
			protocol.WithHTTP3Timeout(cliCfg.Timeout),
			protocol.WithHTTP3UserAgent(fmt.Sprintf("Burkut/%s", version.Version)),
		)
		defer http3Client.Close()
	}

	// Get file info first
	if cliCfg.Verbose {
		fmt.Fprintf(os.Stderr, "Fetching metadata from %s\n", url)
	}

	var meta *protocol.Metadata
	if http3Client != nil {
		meta, err = http3Client.Head(ctx, url)
	} else {
		meta, err = httpClient.Head(ctx, url)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get file info: %v\n", err)
		return ExitNetworkError
	}

	// Determine output path
	outputPath := determineOutputPath(cliCfg, meta.Filename)

	if cliCfg.Verbose {
		fmt.Fprintf(os.Stderr, "Filename: %s\n", meta.Filename)
		fmt.Fprintf(os.Stderr, "Size: %s\n", ui.FormatBytes(meta.ContentLength))
		fmt.Fprintf(os.Stderr, "Resume supported: %v\n", meta.AcceptRanges)
		fmt.Fprintf(os.Stderr, "Protocol: %s\n", meta.Protocol)
		if !meta.LastModified.IsZero() {
			fmt.Fprintf(os.Stderr, "Last-Modified: %s\n", meta.LastModified.Format(time.RFC1123))
		}
		fmt.Fprintf(os.Stderr, "Saving to: %s\n", outputPath)
	}

	// Check timestamping condition
	if cliCfg.Timestamping {
		check, err := engine.CheckTimestamp(outputPath, meta)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking timestamp: %v\n", err)
			return ExitGeneralError
		}

		if !check.ShouldDownload {
			if !cliCfg.Quiet {
				fmt.Printf("File '%s' is up-to-date, skipping download.\n", meta.Filename)
				if cliCfg.Verbose {
					fmt.Fprintf(os.Stderr, "Reason: %s\n", check.Reason)
				}
			}
			return ExitSuccess
		}

		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Download needed: %s\n", check.Reason)
		}
	}

	// Create downloader
	downloaderConfig := engine.DefaultConfig()
	downloaderConfig.Connections = cliCfg.Connections
	if rateLimiter != nil {
		downloaderConfig.RateLimiter = rateLimiter
	}

	downloader := engine.NewDownloader(downloaderConfig, httpClient)

	// Setup progress display
	var progressBar *ui.ProgressBar
	if cliCfg.Progress != "none" {
		progressBar = ui.NewProgressBar(
			ui.WithNoColor(cliCfg.NoColor),
			ui.WithChunks(cliCfg.Verbose),
		)

		downloader.SetProgressCallback(func(p engine.Progress) {
			switch cliCfg.Progress {
			case "bar":
				progressBar.Render(os.Stdout, p, meta.Filename)
			case "minimal":
				ui.MinimalProgress(os.Stdout, p, meta.Filename)
			case "json":
				ui.RenderJSON(os.Stdout, p, meta.Filename)
			}
		})
	}

	// Print header
	if !cliCfg.Quiet && cliCfg.Progress == "bar" {
		fmt.Printf("Burkut %s - Downloading\n\n", version.Version)
	}

	// Start download (with mirror support)
	startTime := time.Now()

	// Parse mirror URLs if provided
	var mirrors []string
	if cliCfg.MirrorURLs != "" {
		for _, m := range strings.Split(cliCfg.MirrorURLs, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				mirrors = append(mirrors, m)
			}
		}
	}

	// Try main URL first, then mirrors
	allURLs := append([]string{url}, mirrors...)
	for i, downloadURL := range allURLs {
		if i > 0 {
			if cliCfg.Verbose || !cliCfg.Quiet {
				fmt.Fprintf(os.Stderr, "Trying mirror %d/%d: %s\n", i, len(mirrors), downloadURL)
			}
		}

		err = downloader.Download(ctx, downloadURL, outputPath)
		if err == nil {
			break // Success
		}

		// Don't try more mirrors if context was cancelled
		if ctx.Err() != nil {
			break
		}

		if cliCfg.Verbose && i < len(allURLs)-1 {
			fmt.Fprintf(os.Stderr, "Failed: %v, trying next mirror...\n", err)
		}
	}

	// Setup hooks
	hookManager := setupHooks(cliCfg)
	elapsed := time.Since(startTime)

	// Handle result
	if err != nil {
		// Execute error hooks
		if hookManager.Count() > 0 {
			payload := hooks.CreatePayload(hooks.EventError, url, meta.Filename, outputPath).
				WithError(err).
				WithDuration(elapsed)
			hookManager.ExecuteAsync(ctx, payload)
		}

		if ctx.Err() == context.Canceled {
			if progressBar != nil {
				progressBar.RenderError(os.Stdout, meta.Filename, fmt.Errorf("interrupted"))
			}
			return ExitInterrupted
		}

		if progressBar != nil {
			progressBar.RenderError(os.Stdout, meta.Filename, err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return ExitNetworkError
	}

	// Auto-verify: try to fetch checksum file if enabled
	if expectedChecksum == nil && cliCfg.AutoVerify {
		// Try common checksum file extensions
		checksumExts := []struct {
			ext string
			alg engine.ChecksumAlgorithm
		}{
			{".sha256", engine.AlgorithmSHA256},
			{".sha256sum", engine.AlgorithmSHA256},
			{".sha512", engine.AlgorithmSHA512},
			{".md5", engine.AlgorithmMD5},
			{".md5sum", engine.AlgorithmMD5},
		}

		for _, cs := range checksumExts {
			checksumURL := url + cs.ext
			if cliCfg.Verbose {
				fmt.Fprintf(os.Stderr, "Trying checksum URL: %s\n", checksumURL)
			}

			// Try to fetch the checksum file
			checksumValue, alg, fetchErr := engine.FetchAndParseChecksumURL(checksumURL, filepath.Base(outputPath), func(fetchURL string) ([]byte, error) {
				resp, _, fetchErr := httpClient.Get(ctx, fetchURL)
				if fetchErr != nil {
					return nil, fetchErr
				}
				defer resp.Close()
				return io.ReadAll(resp)
			})

			if fetchErr == nil && checksumValue != "" {
				expectedChecksum = &engine.Checksum{
					Algorithm: alg,
					Value:     strings.ToLower(checksumValue),
				}
				if !cliCfg.Quiet {
					fmt.Fprintf(os.Stderr, "Found checksum file: %s\n", checksumURL)
				}
				break
			}
		}

		if expectedChecksum == nil && !cliCfg.Quiet {
			fmt.Fprintf(os.Stderr, "Warning: No checksum file found for auto-verify\n")
		}
	}

	// Verify checksum if provided
	if expectedChecksum != nil {
		if cliCfg.Verbose || cliCfg.Progress != "none" {
			fmt.Fprintf(os.Stderr, "Verifying checksum (%s)...\n", expectedChecksum.Algorithm)
		}

		valid, err := engine.VerifyChecksum(outputPath, expectedChecksum)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to verify checksum: %v\n", err)
			return ExitGeneralError
		}

		if !valid {
			actual, _ := engine.CalculateChecksum(outputPath, expectedChecksum.Algorithm)
			fmt.Fprintf(os.Stderr, "Error: Checksum mismatch!\n")
			fmt.Fprintf(os.Stderr, "  Expected: %s\n", expectedChecksum.Value)
			if actual != nil {
				fmt.Fprintf(os.Stderr, "  Actual:   %s\n", actual.Value)
			}
			return ExitChecksumError
		}

		if cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Checksum verified successfully!\n")
		}
	}

	// Set file modification time to match server (for timestamping)
	if !meta.LastModified.IsZero() {
		if err := engine.SetFileModTime(outputPath, meta.LastModified); err != nil {
			if cliCfg.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: Could not set file modification time: %v\n", err)
			}
		}
	}

	// Success
	finalProgress := downloader.GetProgress()
	finalProgress.ElapsedTime = time.Since(startTime)

	// Execute completion hooks
	if hookManager.Count() > 0 {
		payload := hooks.CreatePayload(hooks.EventComplete, url, meta.Filename, outputPath).
			WithProgress(finalProgress.Downloaded, finalProgress.TotalSize, finalProgress.Speed, finalProgress.Percent).
			WithDuration(finalProgress.ElapsedTime)
		hookManager.ExecuteAsync(ctx, payload)
	}

	if progressBar != nil && cliCfg.Progress == "bar" {
		progressBar.RenderComplete(os.Stdout, finalProgress, meta.Filename)
	} else if !cliCfg.Quiet {
		fmt.Printf("\nDownload complete: %s (%s)\n", outputPath, ui.FormatBytes(meta.ContentLength))
	}

	return ExitSuccess
}

// loadConfig loads configuration from file and applies CLI overrides
func loadConfig(cliCfg CLIConfig) (*config.Config, error) {
	var cfg *config.Config
	var err error

	if cliCfg.ConfigFile != "" {
		// Load from specified config file
		cfg = config.DefaultConfig()
		if err = cfg.LoadFile(cliCfg.ConfigFile); err != nil {
			return nil, err
		}
	} else {
		// Try to load from default locations
		cfg, err = config.Load()
		if err != nil {
			return nil, err
		}
	}

	// Apply profile if specified
	if cliCfg.Profile != "" {
		if err = cfg.ApplyProfile(cliCfg.Profile); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// initConfig generates a default configuration file
func initConfig() int {
	configPath, err := config.GetDefaultConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot determine config path: %v\n", err)
		return ExitGeneralError
	}

	// Check if file already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(os.Stderr, "Config file already exists: %s\n", configPath)
		fmt.Fprintf(os.Stderr, "Use --config to specify a different file.\n")
		return ExitGeneralError
	}

	// Create default config
	cfg := config.DefaultConfig()
	if err := cfg.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to save config: %v\n", err)
		return ExitGeneralError
	}

	fmt.Printf("Created default config file: %s\n", configPath)
	fmt.Println("\nYou can customize your settings there.")
	return ExitSuccess
}

func determineOutputPath(cfg CLIConfig, filename string) string {
	// Use custom output name if specified
	if cfg.Output != "" {
		if filepath.IsAbs(cfg.Output) {
			return cfg.Output
		}
		return filepath.Join(cfg.OutputDir, cfg.Output)
	}

	// Use filename from server
	return filepath.Join(cfg.OutputDir, filename)
}

func printUsage() {
	fmt.Printf(`%s

Usage:
  burkut [OPTIONS] URL

A modern download manager combining the best of wget and curl
with HTTP/2 support, parallel downloads, and smart resume.

Options:
  -o, --output FILE      Write output to FILE
  -P, --output-dir DIR   Save files to DIR (default: current directory)
  -c, --continue         Resume partially downloaded file
  -n, --connections N    Number of parallel connections (default: 4)
  -T, --timeout DUR      Connection timeout (default: 30s)
  -q, --quiet            Quiet mode (no progress output)
  -v, --verbose          Verbose output (show headers and chunk info)
      --progress TYPE    Progress display: bar, minimal, json, none (default: bar)
      --no-color         Disable colored output
  -h, --help             Show this help message
  -V, --version          Show version information

Advanced Options:
      --limit-rate RATE  Limit download speed (e.g., 10M, 500K)
      --checksum SUM     Verify file checksum (e.g., sha256:abc123, blake3:def456)
      --verify           Auto-detect and verify checksum (.sha256, .md5 files)
      --proxy URL        Use proxy (http://host:port or socks5://host:port)
      --no-check-certificate  Skip TLS certificate verification
      --config FILE      Use custom config file
      --profile NAME     Use named profile from config
      --init-config      Generate default config file

Authentication Options:
  -u, --user USER:PASS   Basic authentication credentials
      --netrc            Use ~/.netrc for authentication
  -H, --header HEADER    Add custom header (can be repeated)

Protocol Options:
      --http1            Force HTTP/1.1 (disable HTTP/2)
      --http2            Force HTTP/2 (fail if server doesn't support)
      --http3            Use HTTP/3 (QUIC) protocol (experimental)

Conditional Download:
  -N, --timestamping     Only download if remote file is newer than local

Security Options:
      --pinnedpubkey PIN SHA256 public key pin (sha256//base64hash)

Batch & Automation:
  -i, --input-file FILE  Read URLs from file (batch download)
      --on-complete CMD  Run command after successful download
      --on-error CMD     Run command after failed download
      --webhook URL      Send webhook notification on complete/error
      --mirrors URLs     Comma-separated mirror URLs for fallback

Interface:
      --tui              Use interactive TUI mode (fullscreen)

Recursive Download (Spider Mode):
  -r, --recursive        Enable recursive download
  -l, --level DEPTH      Maximum recursion depth (default: 5, 0 = unlimited)
  -m, --mirror           Mirror mode (-r -N -l 0 -p)
      --span-hosts       Follow links to other hosts
  -A, --accept PATTERNS  Accept patterns (comma-separated, e.g., '*.pdf,*.doc')
  -R, --reject PATTERNS  Reject patterns (comma-separated)
      --accept-ext EXTS  Accept extensions (comma-separated)
      --reject-ext EXTS  Reject extensions (comma-separated)
  -p, --page-requisites  Download page requisites (CSS, JS, images)
  -k, --convert-links    Convert links to local paths
      --spider           Spider mode: list URLs only (no download)
  -w, --wait TIME        Wait time between requests (e.g., '1s')
      --random-wait      Add random 0-500ms to wait time
  -e, --robots-off       Ignore robots.txt

Monitoring:
      --metrics-addr ADDR  Prometheus metrics endpoint (e.g., :9090)

Exit Codes:
  0  Success
  1  General error
  2  Parse/config error
  3  Network error
  4  Authentication error
  5  TLS/SSL error
  6  Checksum mismatch
  7  Timeout
  8  Interrupted (Ctrl+C)

Examples:
  burkut https://example.com/file.zip
  burkut -o myfile.zip https://example.com/file.zip
  burkut -c https://example.com/large-file.iso
  burkut -n 8 https://example.com/large-file.iso
  burkut --limit-rate 1M https://example.com/large.iso
  burkut --checksum sha256:abc123... https://example.com/file.zip
  burkut --checksum blake3:def456... https://example.com/file.zip
  burkut --proxy http://proxy:8080 https://example.com/file.zip
  burkut --proxy socks5://127.0.0.1:9050 https://example.com/file.zip
  burkut --profile fast https://example.com/file.zip
  burkut --mirrors "https://mirror1.com/f.zip,https://mirror2.com/f.zip" https://example.com/file.zip
  burkut -u admin:secret https://example.com/protected/file.zip
  burkut --netrc https://example.com/file.zip
  burkut -H "X-API-Key: abc123" https://api.example.com/download
  burkut --tui https://example.com/large-file.iso

Batch Download:
  burkut -i urls.txt                   Download all URLs from file
  burkut -i urls.txt -P /downloads     Download to specific directory

Spider Mode (list URLs without downloading):
  burkut --spider https://example.com/docs/           List all URLs
  burkut --spider -l 3 https://example.com/ > urls.txt  Save to file

URL File Format (one URL per line):
  https://example.com/file1.zip
  https://example.com/file2.zip output.zip
  https://example.com/file3.zip|custom.zip|sha256:abc123

For more information, visit: https://github.com/kilimcininkoroglu/burkut
`, version.Full())
}

// runBatchDownload processes a file containing URLs
func runBatchDownload(cliCfg CLIConfig) int {
	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nInterrupted, stopping downloads...")
		cancel()
	}()

	// Load config
	cfg, err := loadConfig(cliCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return ExitParseError
	}

	// Create queue and load URLs
	queue := download.NewQueue(cliCfg.OutputDir)
	if err := queue.LoadFromFile(cliCfg.InputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading URL file: %v\n", err)
		return ExitParseError
	}

	if queue.Count() == 0 {
		fmt.Fprintln(os.Stderr, "No URLs found in file")
		return ExitParseError
	}

	fmt.Printf("Burkut %s - Batch Download\n", version.Version)
	fmt.Printf("Loaded %d URLs from %s\n\n", queue.Count(), cliCfg.InputFile)

	// Build HTTP client options
	httpOpts := buildHTTPOptions(cliCfg, cfg)

	// Create HTTP client
	httpClient := protocol.NewHTTPClient(httpOpts...)

	// Setup rate limiter
	var rateLimiter *engine.RateLimiter
	if cliCfg.LimitRate != "" {
		bytesPerSec, err := config.ParseBandwidth(cliCfg.LimitRate)
		if err == nil && bytesPerSec > 0 {
			rateLimiter = engine.NewRateLimiter(bytesPerSec)
		}
	}

	// Process each URL
	completed := 0
	failed := 0
	startTime := time.Now()

	items := queue.Items()
	for i, item := range items {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\nBatch download interrupted\n")
			return ExitInterrupted
		default:
		}

		fmt.Printf("[%d/%d] %s\n", i+1, len(items), item.URL)
		queue.UpdateStatus(item.ID, download.QueueStatusDownloading)

		// Create downloader config
		dlConfig := engine.DefaultConfig()
		dlConfig.Connections = cliCfg.Connections
		if rateLimiter != nil {
			dlConfig.RateLimiter = rateLimiter
		}

		downloader := engine.NewDownloader(dlConfig, httpClient)

		// Setup minimal progress for batch mode
		if !cliCfg.Quiet {
			downloader.SetProgressCallback(func(p engine.Progress) {
				if cliCfg.Progress == "bar" {
					fmt.Printf("\r  %.1f%% %s/%s %s/s",
						p.Percent,
						ui.FormatBytes(p.Downloaded),
						ui.FormatBytes(p.TotalSize),
						ui.FormatBytes(p.Speed))
				}
			})
		}

		// Download
		err := downloader.Download(ctx, item.URL, item.OutputPath)

		if err != nil {
			queue.SetError(item.ID, err)
			failed++
			if !cliCfg.Quiet {
				fmt.Printf("\r  ✗ Failed: %v\n", err)
			}
			continue
		}

		// Verify checksum if provided
		if item.Checksum != "" {
			checksum, parseErr := engine.ParseChecksumAuto(item.Checksum)
			if parseErr == nil {
				valid, verifyErr := engine.VerifyChecksum(item.OutputPath, checksum)
				if verifyErr != nil || !valid {
					queue.SetError(item.ID, fmt.Errorf("checksum mismatch"))
					failed++
					if !cliCfg.Quiet {
						fmt.Printf("\r  ✗ Checksum mismatch\n")
					}
					continue
				}
			}
		}

		queue.UpdateStatus(item.ID, download.QueueStatusCompleted)
		completed++
		if !cliCfg.Quiet {
			fmt.Printf("\r  ✓ Completed\n")
		}
	}

	// Print summary
	elapsed := time.Since(startTime)
	fmt.Printf("\n")
	fmt.Printf("Batch download complete:\n")
	fmt.Printf("  Total:     %d\n", len(items))
	fmt.Printf("  Completed: %d\n", completed)
	fmt.Printf("  Failed:    %d\n", failed)
	fmt.Printf("  Time:      %s\n", elapsed.Round(time.Second))

	if failed > 0 {
		return ExitGeneralError
	}
	return ExitSuccess
}

// buildHTTPOptions creates HTTP client options from CLI and config
func buildHTTPOptions(cliCfg CLIConfig, cfg *config.Config) []protocol.HTTPClientOption {
	opts := []protocol.HTTPClientOption{
		protocol.WithTimeout(cliCfg.Timeout),
		protocol.WithUserAgent(fmt.Sprintf("Burkut/%s", version.Version)),
	}

	// Add proxy
	proxyURL := cliCfg.Proxy
	if proxyURL == "" && cfg.Proxy.HTTP != "" {
		proxyURL = cfg.Proxy.HTTP
	}
	if proxyURL != "" {
		if strings.HasPrefix(proxyURL, "socks5://") {
			opts = append(opts, protocol.WithSOCKS5Proxy(proxyURL, nil))
		} else {
			opts = append(opts, protocol.WithProxy(proxyURL))
		}
	}

	// Skip certificate verification
	if cliCfg.NoCheckCert || !cfg.TLS.Verify {
		opts = append(opts, protocol.WithInsecureSkipVerify(true))
	}

	return opts
}

// setupHooks creates a hook manager from CLI options
func setupHooks(cliCfg CLIConfig) *hooks.Manager {
	manager := hooks.NewManager()

	// Add command hooks
	if cliCfg.OnComplete != "" {
		manager.AddCommand(cliCfg.OnComplete, hooks.EventComplete)
	}
	if cliCfg.OnError != "" {
		manager.AddCommand(cliCfg.OnError, hooks.EventError)
	}

	// Add webhook
	if cliCfg.WebhookURL != "" {
		manager.AddWebhook(cliCfg.WebhookURL, hooks.EventComplete, hooks.EventError)
	}

	return manager
}

// runTorrentDownload downloads a torrent file or magnet link
func runTorrentDownload(cliCfg CLIConfig, source string) int {
	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nInterrupted, shutting down...")
		cancel()
	}()

	// Configure torrent client
	torrentCfg := btorrent.DefaultConfig()
	torrentCfg.DownloadDir = cliCfg.OutputDir

	// Apply rate limit if specified
	if cliCfg.LimitRate != "" {
		if limit, err := config.ParseBandwidth(cliCfg.LimitRate); err == nil {
			torrentCfg.DownloadLimit = limit
		}
	}

	// Create client
	client, err := btorrent.NewClient(torrentCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating torrent client: %v\n", err)
		return ExitGeneralError
	}
	defer client.Close()

	// Add torrent
	var dl *btorrent.Download
	if btorrent.IsMagnetURI(source) {
		if !cliCfg.Quiet {
			fmt.Fprintln(os.Stderr, "Adding magnet link, fetching metadata...")
		}
		dl, err = client.AddMagnet(source)
	} else {
		if !cliCfg.Quiet {
			fmt.Fprintf(os.Stderr, "Loading torrent file: %s\n", source)
		}
		dl, err = client.AddTorrentFile(source)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding torrent: %v\n", err)
		return ExitGeneralError
	}

	if !cliCfg.Quiet {
		fmt.Fprintf(os.Stderr, "Downloading: %s\n", dl.Name)
		fmt.Fprintf(os.Stderr, "Size: %s\n", formatBytes(dl.TotalSize))
		fmt.Fprintf(os.Stderr, "Info Hash: %s\n", dl.InfoHash)
	}

	// Progress callback
	progressCb := func(p btorrent.Progress) {
		if cliCfg.Quiet {
			return
		}
		switch cliCfg.Progress {
		case "bar", "minimal":
			fmt.Fprintf(os.Stderr, "\r\033[K%s: %.1f%% [%s/%s] %s/s P:%d S:%d",
				p.Name,
				p.Percent,
				formatBytes(p.Downloaded),
				formatBytes(p.TotalSize),
				formatBytes(p.Speed),
				p.Peers,
				p.Seeds)
		case "json":
			fmt.Printf(`{"name":"%s","percent":%.1f,"downloaded":%d,"total":%d,"speed":%d,"peers":%d,"seeds":%d,"status":"%s"}`+"\n",
				p.Name, p.Percent, p.Downloaded, p.TotalSize, p.Speed, p.Peers, p.Seeds, p.Status)
		}
	}

	// Download
	err = client.Download(ctx, dl, progressCb)

	if err != nil {
		if ctx.Err() != nil {
			return ExitInterrupted
		}
		fmt.Fprintf(os.Stderr, "Error downloading: %v\n", err)
		return ExitNetworkError
	}

	if !cliCfg.Quiet {
		fmt.Fprintf(os.Stderr, "\nDownload complete: %s\n", dl.Name)
	}

	// Run hooks
	hookManager := setupHooks(cliCfg)
	if hookManager.Count() > 0 {
		elapsed := time.Since(dl.StartTime)
		payload := hooks.CreatePayload(hooks.EventComplete, source, dl.Name, filepath.Join(cliCfg.OutputDir, dl.Name)).
			WithProgress(dl.Downloaded, dl.TotalSize, 0, 100.0).
			WithDuration(elapsed)
		hookManager.ExecuteAsync(ctx, payload)
	}

	return ExitSuccess
}

// runDownloadTUI runs the download with interactive TUI
func runDownloadTUI(cliCfg CLIConfig, url string) int {
	// Load config
	cfg, err := loadConfig(cliCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return ExitParseError
	}

	// Build HTTP client options
	httpOpts := []protocol.HTTPClientOption{
		protocol.WithTimeout(cliCfg.Timeout),
		protocol.WithUserAgent(fmt.Sprintf("Burkut/%s", version.Version)),
	}

	// Add proxy
	proxyURL := cliCfg.Proxy
	if proxyURL == "" && cfg.Proxy.HTTP != "" {
		proxyURL = cfg.Proxy.HTTP
	}
	if proxyURL != "" {
		if strings.HasPrefix(proxyURL, "socks5://") {
			httpOpts = append(httpOpts, protocol.WithSOCKS5Proxy(proxyURL, nil))
		} else {
			httpOpts = append(httpOpts, protocol.WithProxy(proxyURL))
		}
	}

	// Skip certificate verification
	if cliCfg.NoCheckCert || !cfg.TLS.Verify {
		httpOpts = append(httpOpts, protocol.WithInsecureSkipVerify(true))
	}

	// Handle authentication
	if cliCfg.UseNetrc {
		netrc, err := config.LoadNetrc()
		if err == nil {
			if login, password, found := netrc.GetCredentials(url); found {
				httpOpts = append(httpOpts, protocol.WithBasicAuth(login, password))
			}
		}
	}
	if cliCfg.BasicAuth != "" {
		parts := strings.SplitN(cliCfg.BasicAuth, ":", 2)
		password := ""
		if len(parts) > 1 {
			password = parts[1]
		}
		httpOpts = append(httpOpts, protocol.WithBasicAuth(parts[0], password))
	}

	// Custom headers
	if len(cliCfg.Headers) > 0 {
		headers := make(map[string]string)
		for _, h := range cliCfg.Headers {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) == 2 {
				headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		httpOpts = append(httpOpts, protocol.WithHeaders(headers))
	}

	// Create HTTP client
	httpClient := protocol.NewHTTPClient(httpOpts...)

	// Get file metadata
	meta, err := httpClient.Head(context.Background(), url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get file info: %v\n", err)
		return ExitNetworkError
	}

	// Determine output path
	outputPath := determineOutputPath(cliCfg, meta.Filename)

	// Create TUI runner
	tuiRunner := tui.NewRunner(url, meta.Filename, meta.ContentLength, cliCfg.Connections)

	// Create downloader
	downloaderConfig := engine.DefaultConfig()
	downloaderConfig.Connections = cliCfg.Connections

	// Rate limiter
	if cliCfg.LimitRate != "" {
		bytesPerSec, err := config.ParseBandwidth(cliCfg.LimitRate)
		if err == nil {
			downloaderConfig.RateLimiter = engine.NewRateLimiter(bytesPerSec)
		}
	}

	downloader := engine.NewDownloader(downloaderConfig, httpClient)

	// Set progress callback for TUI
	downloader.SetProgressCallback(func(p engine.Progress) {
		tuiRunner.UpdateProgress(p, p.ChunkStatus)
	})

	// Run download in background
	var downloadErr error
	var downloadDone = make(chan struct{})

	go func() {
		defer close(downloadDone)
		downloadErr = downloader.Download(tuiRunner.Context(), url, outputPath)
	}()

	// Start TUI (blocks until user quits or download completes)
	go func() {
		<-downloadDone
		if downloadErr != nil {
			tuiRunner.SetError(downloadErr)
		} else {
			progress := downloader.GetProgress()
			tuiRunner.SetComplete(meta.Filename, meta.ContentLength, progress.ElapsedTime, progress.Speed)

			// Verify checksum if specified
			if cliCfg.Checksum != "" {
				tuiRunner.SetVerifying()
				expectedChecksum, _ := engine.ParseChecksumAuto(cliCfg.Checksum)
				if expectedChecksum != nil {
					valid, _ := engine.VerifyChecksum(outputPath, expectedChecksum)
					tuiRunner.SetVerified(valid)
				}
			}
		}

		// Wait a bit to show final status
		time.Sleep(2 * time.Second)
		tuiRunner.Stop()
	}()

	// Run the TUI
	if err := tuiRunner.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		return ExitGeneralError
	}

	if downloadErr != nil {
		return ExitNetworkError
	}

	return ExitSuccess
}

func runRecursiveDownload(cliCfg CLIConfig, startURL string) int {
	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted, stopping crawler...")
		cancel()
	}()

	// Determine output directory
	outputDir := cliCfg.Output
	if outputDir == "" {
		outputDir = "."
	}

	// Create crawler config
	crawlConfig := crawler.DefaultConfig()
	crawlConfig.MaxDepth = cliCfg.MaxDepth
	crawlConfig.OutputDir = outputDir
	crawlConfig.RespectRobots = !cliCfg.RobotsOff
	crawlConfig.ConvertLinks = cliCfg.ConvertLinks
	crawlConfig.Workers = cliCfg.Connections
	if crawlConfig.Workers <= 0 {
		crawlConfig.Workers = 2
	}

	// Set user agent
	crawlConfig.UserAgent = "Burkut/1.0 (Website Mirror; +https://github.com/kilimcininkoroglu/burkut)"

	// Parse wait time
	if cliCfg.WaitTime != "" {
		if waitDuration, err := time.ParseDuration(cliCfg.WaitTime); err == nil {
			crawlConfig.WaitTime = waitDuration
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Invalid wait time %q, using default\n", cliCfg.WaitTime)
		}
	}

	// Random wait
	if cliCfg.RandomWait {
		crawlConfig.RandomWait = 500 * time.Millisecond
	}

	// Convert links option
	crawlConfig.ConvertLinks = cliCfg.ConvertLinks

	// Create filter
	filter := &crawler.Filter{
		SameDomain:     !cliCfg.SpanHosts,
		FollowExternal: cliCfg.SpanHosts,
		PageRequisites: cliCfg.PageRequisites,
	}

	// Set patterns
	if cliCfg.AcceptPatterns != "" {
		filter.SetAcceptPatterns(cliCfg.AcceptPatterns)
	}
	if cliCfg.RejectPatterns != "" {
		filter.SetRejectPatterns(cliCfg.RejectPatterns)
	}
	if cliCfg.AcceptExt != "" {
		filter.SetAcceptExtensions(cliCfg.AcceptExt)
	}
	if cliCfg.RejectExt != "" {
		filter.SetRejectExtensions(cliCfg.RejectExt)
	}

	crawlConfig.Filter = filter

	// Create crawler
	c := crawler.NewCrawler(crawlConfig)

	// Progress callback
	if !cliCfg.Quiet {
		lastReport := time.Time{}
		c.SetProgressCallback(func(current, total int, url string, status crawler.Status) {
			// Rate limit output
			if time.Since(lastReport) < 500*time.Millisecond {
				return
			}
			lastReport = time.Now()

			stats := c.GetStats()
			elapsed := time.Since(stats.StartTime).Round(time.Second)
			fmt.Fprintf(os.Stderr, "\r[%s] Downloaded: %d | Failed: %d | Skipped: %d | Depth: %d | %s",
				elapsed,
				stats.DownloadedURLs,
				stats.FailedURLs,
				stats.SkippedURLs,
				stats.CurrentDepth,
				truncateURL(url, 50),
			)
		})
	}

	// Error callback
	if cliCfg.Verbose {
		c.SetErrorCallback(func(url string, err error) {
			fmt.Fprintf(os.Stderr, "\nError: %s: %v\n", url, err)
		})
	}

	// Print start message
	if !cliCfg.Quiet {
		fmt.Fprintf(os.Stderr, "Starting recursive download from: %s\n", startURL)
		fmt.Fprintf(os.Stderr, "Output directory: %s\n", outputDir)
		fmt.Fprintf(os.Stderr, "Max depth: %d, Workers: %d\n", cliCfg.MaxDepth, crawlConfig.Workers)
		if cliCfg.RobotsOff {
			fmt.Fprintln(os.Stderr, "Note: robots.txt checking disabled")
		}
		fmt.Fprintln(os.Stderr)
	}

	// Start crawling
	err := c.Crawl(ctx, startURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nCrawl error: %v\n", err)
		return ExitNetworkError
	}

	// Print final stats
	stats := c.GetStats()
	if !cliCfg.Quiet {
		fmt.Fprintf(os.Stderr, "\n\nCrawl completed!\n")
		fmt.Fprintf(os.Stderr, "Total URLs: %d\n", stats.TotalURLs)
		fmt.Fprintf(os.Stderr, "Downloaded: %d\n", stats.DownloadedURLs)
		fmt.Fprintf(os.Stderr, "Failed: %d\n", stats.FailedURLs)
		fmt.Fprintf(os.Stderr, "Skipped: %d\n", stats.SkippedURLs)
		fmt.Fprintf(os.Stderr, "Total bytes: %s\n", formatBytes(stats.TotalBytes))
		fmt.Fprintf(os.Stderr, "Elapsed time: %s\n", stats.EndTime.Sub(stats.StartTime).Round(time.Second))
	}

	if stats.FailedURLs > 0 {
		return ExitGeneralError
	}

	return ExitSuccess
}

// runSpiderMode runs spider mode - lists URLs without downloading
func runSpiderMode(cliCfg CLIConfig, startURL string) int {
	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted, stopping...")
		cancel()
	}()

	// Create crawler config (similar to recursive mode)
	crawlConfig := &crawler.Config{
		MaxDepth:      cliCfg.MaxDepth,
		Workers:       cliCfg.Connections,
		OutputDir:     "", // No output needed
		RespectRobots: !cliCfg.RobotsOff,
		DownloadFiles: false, // Spider mode - don't download
	}

	// Set defaults
	if crawlConfig.MaxDepth <= 0 {
		crawlConfig.MaxDepth = 5
	}
	if crawlConfig.Workers <= 0 {
		crawlConfig.Workers = 2
	}

	crawlConfig.UserAgent = "Burkut/1.0 Spider (URL checker; +https://github.com/kilimcininkoroglu/burkut)"

	// Parse wait time
	if cliCfg.WaitTime != "" {
		if waitDuration, err := time.ParseDuration(cliCfg.WaitTime); err == nil {
			crawlConfig.WaitTime = waitDuration
		}
	}

	// Create filter
	filter := &crawler.Filter{
		SameDomain:     !cliCfg.SpanHosts,
		FollowExternal: cliCfg.SpanHosts,
		PageRequisites: cliCfg.PageRequisites,
	}

	// Set patterns
	if cliCfg.AcceptPatterns != "" {
		filter.AcceptPatterns = strings.Split(cliCfg.AcceptPatterns, ",")
	}
	if cliCfg.RejectPatterns != "" {
		filter.RejectPatterns = strings.Split(cliCfg.RejectPatterns, ",")
	}
	if cliCfg.AcceptExt != "" {
		filter.AcceptExtensions = strings.Split(cliCfg.AcceptExt, ",")
	}
	if cliCfg.RejectExt != "" {
		filter.RejectExtensions = strings.Split(cliCfg.RejectExt, ",")
	}

	crawlConfig.Filter = filter

	// Create crawler
	c := crawler.NewCrawler(crawlConfig)

	// Track found URLs
	foundURLs := make(map[string]bool)
	var urlMutex sync.Mutex
	var urlCount int

	// URL found callback - print each URL as found
	c.SetProgressCallback(func(current, total int, url string, status crawler.Status) {
		if status == crawler.StatusCompleted || status == crawler.StatusDownloading {
			urlMutex.Lock()
			if !foundURLs[url] {
				foundURLs[url] = true
				urlCount++
				// Print URL to stdout (can be piped)
				fmt.Println(url)
			}
			urlMutex.Unlock()
		}
	})

	// Print header to stderr (so stdout is clean for piping)
	if !cliCfg.Quiet {
		fmt.Fprintf(os.Stderr, "Spider mode: crawling %s (max depth: %d)\n", startURL, cliCfg.MaxDepth)
		fmt.Fprintln(os.Stderr, "URLs found:")
		fmt.Fprintln(os.Stderr, "---")
	}

	// Start crawling
	err := c.Crawl(ctx, startURL)
	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "\nSpider error: %v\n", err)
		return ExitNetworkError
	}

	// Print summary to stderr
	stats := c.GetStats()
	if !cliCfg.Quiet {
		fmt.Fprintln(os.Stderr, "---")
		fmt.Fprintf(os.Stderr, "Total URLs found: %d\n", urlCount)
		fmt.Fprintf(os.Stderr, "Pages crawled: %d\n", stats.DownloadedURLs)
		fmt.Fprintf(os.Stderr, "Failed: %d\n", stats.FailedURLs)
	}

	return ExitSuccess
}

// truncateURL truncates a URL for display
func truncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	return url[:maxLen-3] + "..."
}

// formatBytes formats bytes to human-readable string
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// runMetalinkDownload downloads a file using a Metalink file
func runMetalinkDownload(cliCfg CLIConfig, metalinkFile string) int {
	// Parse metalink file
	ml, err := metalink.ParseFile(metalinkFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing metalink file: %v\n", err)
		return ExitParseError
	}

	// Get the first file (or handle multiple files)
	file := ml.GetFile()
	if file == nil {
		fmt.Fprintln(os.Stderr, "Error: No files found in metalink")
		return ExitParseError
	}

	// Print metalink info
	if !cliCfg.Quiet {
		fmt.Fprintf(os.Stderr, "Metalink file: %s\n", metalinkFile)
		fmt.Fprintf(os.Stderr, "File: %s\n", file.Name)
		if file.Size > 0 {
			fmt.Fprintf(os.Stderr, "Size: %s\n", formatBytes(file.Size))
		}
		if file.Description != "" {
			fmt.Fprintf(os.Stderr, "Description: %s\n", file.Description)
		}

		// Show checksum info
		hashType, hashValue := file.GetPreferredChecksum()
		if hashValue != "" {
			fmt.Fprintf(os.Stderr, "Checksum: %s:%s...\n", hashType, hashValue[:min(16, len(hashValue))])
		}

		fmt.Fprintf(os.Stderr, "URLs: %d available\n", len(file.URLs))
		fmt.Fprintln(os.Stderr)
	}

	// Get sorted URLs by priority
	urls := file.SortedURLs()
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No download URLs in metalink")
		return ExitParseError
	}

	// Set output filename from metalink if not specified
	if cliCfg.Output == "" {
		cliCfg.Output = file.Name
	}

	// Set checksum from metalink if not specified
	if cliCfg.Checksum == "" {
		hashType, hashValue := file.GetPreferredChecksum()
		if hashValue != "" {
			// Convert metalink hash type to our format
			switch hashType {
			case "sha-256":
				cliCfg.Checksum = "sha256:" + hashValue
			case "sha-512":
				cliCfg.Checksum = "sha512:" + hashValue
			case "sha-1":
				cliCfg.Checksum = "sha1:" + hashValue
			case "md5":
				cliCfg.Checksum = "md5:" + hashValue
			}
		}
	}

	// Try URLs in priority order
	var lastErr error
	for i, urlEntry := range urls {
		if !cliCfg.Quiet && cliCfg.Verbose {
			fmt.Fprintf(os.Stderr, "Trying URL %d/%d: %s\n", i+1, len(urls), urlEntry.URL)
		}

		exitCode := runDownload(cliCfg, urlEntry.URL)
		if exitCode == ExitSuccess {
			return ExitSuccess
		}

		if !cliCfg.Quiet {
			fmt.Fprintf(os.Stderr, "URL failed, trying next mirror...\n")
		}
		lastErr = fmt.Errorf("download failed with exit code %d", exitCode)
	}

	fmt.Fprintf(os.Stderr, "Error: All mirrors failed. Last error: %v\n", lastErr)
	return ExitNetworkError
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isFTPURL checks if URL is FTP or FTPS
func isFTPURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "ftp" || scheme == "ftps"
}

// runFTPDownload handles FTP/FTPS downloads
func runFTPDownload(cliCfg CLIConfig, rawURL string) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nInterrupted...")
		cancel()
	}()

	// Parse URL to check scheme
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid URL: %v\n", err)
		return ExitParseError
	}

	// Build FTP client options
	ftpOpts := []protocol.FTPClientOption{
		protocol.WithFTPTimeout(cliCfg.Timeout),
	}

	// Enable TLS for ftps:// scheme
	if strings.ToLower(parsedURL.Scheme) == "ftps" {
		// Check if using implicit FTPS port (990)
		port := parsedURL.Port()
		if port == "990" || port == "" {
			// Default FTPS port or explicit 990 = implicit TLS
			ftpOpts = append(ftpOpts, protocol.WithFTPSImplicit(true))
			if cliCfg.Verbose {
				fmt.Fprintln(os.Stderr, "Using implicit FTPS (port 990)")
			}
		} else {
			// Explicit FTPS (AUTH TLS) on non-standard port
			ftpOpts = append(ftpOpts, protocol.WithFTPS(true))
			if cliCfg.Verbose {
				fmt.Fprintln(os.Stderr, "Using explicit FTPS (AUTH TLS)")
			}
		}
		if cliCfg.NoCheckCert {
			ftpOpts = append(ftpOpts, protocol.WithFTPSkipTLSVerify(true))
		}
	}

	// Handle authentication from URL or CLI
	if parsedURL.User != nil {
		username := parsedURL.User.Username()
		password, _ := parsedURL.User.Password()
		ftpOpts = append(ftpOpts, protocol.WithFTPAuth(username, password))
	} else if cliCfg.BasicAuth != "" {
		parts := strings.SplitN(cliCfg.BasicAuth, ":", 2)
		if len(parts) == 2 {
			ftpOpts = append(ftpOpts, protocol.WithFTPAuth(parts[0], parts[1]))
		}
	}

	// Create FTP client
	ftpClient := protocol.NewFTPClient(ftpOpts...)

	// Get file info
	if cliCfg.Verbose {
		fmt.Fprintf(os.Stderr, "Connecting to %s\n", parsedURL.Host)
	}

	meta, err := ftpClient.Head(ctx, rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get file info: %v\n", err)
		return ExitNetworkError
	}

	// Determine output path
	outputPath := determineOutputPath(cliCfg, meta.Filename)

	if !cliCfg.Quiet {
		fmt.Fprintf(os.Stderr, "File: %s\n", meta.Filename)
		fmt.Fprintf(os.Stderr, "Size: %s\n", formatBytes(meta.ContentLength))
		if !meta.LastModified.IsZero() {
			fmt.Fprintf(os.Stderr, "Modified: %s\n", meta.LastModified.Format(time.RFC1123))
		}
		fmt.Fprintf(os.Stderr, "Saving to: %s\n", outputPath)
		fmt.Fprintln(os.Stderr)
	}

	// Download file
	reader, _, err := ftpClient.Get(ctx, rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to download: %v\n", err)
		return ExitNetworkError
	}
	defer reader.Close()

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create file: %v\n", err)
		return ExitGeneralError
	}
	defer outFile.Close()

	// Copy with progress
	startTime := time.Now()
	var downloaded int64
	showProgress := cliCfg.Progress != "none" && !cliCfg.Quiet
	lastProgressTime := time.Time{}

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ExitInterrupted
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			_, writeErr := outFile.Write(buf[:n])
			if writeErr != nil {
				fmt.Fprintf(os.Stderr, "\nError writing file: %v\n", writeErr)
				return ExitGeneralError
			}
			downloaded += int64(n)

			// Show progress every 100ms
			if showProgress && time.Since(lastProgressTime) > 100*time.Millisecond {
				lastProgressTime = time.Now()
				elapsed := time.Since(startTime)
				speed := int64(0)
				if elapsed.Seconds() > 0 {
					speed = int64(float64(downloaded) / elapsed.Seconds())
				}
				percent := float64(downloaded) / float64(meta.ContentLength) * 100
				fmt.Fprintf(os.Stderr, "\r%s / %s (%.1f%%) %s/s    ",
					formatBytes(downloaded),
					formatBytes(meta.ContentLength),
					percent,
					formatBytes(speed))
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError reading: %v\n", err)
			return ExitNetworkError
		}
	}

	if showProgress {
		fmt.Fprintln(os.Stderr) // New line after progress
	}

	// Print summary
	if !cliCfg.Quiet {
		elapsed := time.Since(startTime)
		speed := int64(float64(downloaded) / elapsed.Seconds())
		fmt.Fprintf(os.Stderr, "\nDownloaded %s in %s (%s/s)\n",
			formatBytes(downloaded),
			elapsed.Round(time.Millisecond),
			formatBytes(speed))
	}

	// Verify checksum if specified
	if cliCfg.Checksum != "" {
		expectedChecksum, err := engine.ParseChecksumAuto(cliCfg.Checksum)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid checksum: %v\n", err)
			return ExitChecksumError
		}

		if !cliCfg.Quiet {
			fmt.Fprintf(os.Stderr, "Verifying checksum...")
		}

		valid, err := engine.VerifyChecksum(outputPath, expectedChecksum)
		if err != nil {
			fmt.Fprintf(os.Stderr, " error: %v\n", err)
			return ExitChecksumError
		}

		if !valid {
			fmt.Fprintf(os.Stderr, " FAILED!\n")
			return ExitChecksumError
		}

		if !cliCfg.Quiet {
			fmt.Fprintf(os.Stderr, " OK\n")
		}
	}

	return ExitSuccess
}
