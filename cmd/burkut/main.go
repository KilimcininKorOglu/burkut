// Burkut - Modern Download Manager
// Named after the golden eagle (berkut) in Turkish mythology
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/config"
	"github.com/kilimcininkoroglu/burkut/internal/download"
	"github.com/kilimcininkoroglu/burkut/internal/engine"
	"github.com/kilimcininkoroglu/burkut/internal/hooks"
	"github.com/kilimcininkoroglu/burkut/internal/protocol"
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
}

func main() {
	cliConfig := parseFlags()

	if cliConfig.ShowVersion {
		fmt.Println(version.Full())
		os.Exit(ExitSuccess)
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
	url := flag.Arg(0)
	if url == "" {
		fmt.Fprintln(os.Stderr, "Error: URL is required")
		printUsage()
		os.Exit(ExitParseError)
	}

	// Run download
	exitCode := runDownload(cliConfig, url)
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

	// Create HTTP client
	httpClient := protocol.NewHTTPClient(httpOpts...)

	// Get file info first
	if cliCfg.Verbose {
		fmt.Fprintf(os.Stderr, "Fetching metadata from %s\n", url)
	}

	meta, err := httpClient.Head(ctx, url)
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
		fmt.Fprintf(os.Stderr, "Saving to: %s\n", outputPath)
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
      --checksum SUM     Verify file checksum (e.g., sha256:abc123... or just abc123)
      --proxy URL        Use proxy (http://host:port or socks5://host:port)
      --no-check-certificate  Skip TLS certificate verification
      --config FILE      Use custom config file
      --profile NAME     Use named profile from config
      --init-config      Generate default config file
  -i, --input-file FILE  Read URLs from file (batch download)
      --on-complete CMD  Run command after successful download
      --on-error CMD     Run command after failed download
      --webhook URL      Send webhook notification on complete/error
      --mirrors URLs     Comma-separated mirror URLs for fallback

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
  burkut --proxy http://proxy:8080 https://example.com/file.zip
  burkut --proxy socks5://127.0.0.1:9050 https://example.com/file.zip
  burkut --profile fast https://example.com/file.zip
  burkut --mirrors "https://mirror1.com/f.zip,https://mirror2.com/f.zip" https://example.com/file.zip

Batch Download:
  burkut -i urls.txt                   Download all URLs from file
  burkut -i urls.txt -P /downloads     Download to specific directory

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
