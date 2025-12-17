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
	"syscall"
	"time"

	"github.com/kilimcininkoroglu/burkut/internal/engine"
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

// Config holds CLI configuration
type Config struct {
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
}

func main() {
	config := parseFlags()

	if config.ShowVersion {
		fmt.Println(version.Full())
		os.Exit(ExitSuccess)
	}

	if config.ShowHelp || flag.NArg() == 0 {
		printUsage()
		if flag.NArg() == 0 && !config.ShowHelp {
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
	exitCode := runDownload(config, url)
	os.Exit(exitCode)
}

func parseFlags() Config {
	config := Config{}

	flag.StringVar(&config.Output, "o", "", "Output filename")
	flag.StringVar(&config.Output, "output", "", "Output filename")
	flag.StringVar(&config.OutputDir, "P", ".", "Output directory")
	flag.StringVar(&config.OutputDir, "output-dir", ".", "Output directory")
	flag.BoolVar(&config.Continue, "c", false, "Continue/resume download")
	flag.BoolVar(&config.Continue, "continue", false, "Continue/resume download")
	flag.IntVar(&config.Connections, "n", 4, "Number of parallel connections")
	flag.IntVar(&config.Connections, "connections", 4, "Number of parallel connections")
	flag.BoolVar(&config.Quiet, "q", false, "Quiet mode (no progress)")
	flag.BoolVar(&config.Quiet, "quiet", false, "Quiet mode (no progress)")
	flag.BoolVar(&config.Verbose, "v", false, "Verbose output")
	flag.BoolVar(&config.Verbose, "verbose", false, "Verbose output")
	flag.BoolVar(&config.NoColor, "no-color", false, "Disable colored output")
	flag.StringVar(&config.Progress, "progress", "bar", "Progress style: bar, minimal, json, none")
	flag.DurationVar(&config.Timeout, "T", 30*time.Second, "Connection timeout")
	flag.DurationVar(&config.Timeout, "timeout", 30*time.Second, "Connection timeout")
	flag.BoolVar(&config.ShowVersion, "V", false, "Show version")
	flag.BoolVar(&config.ShowVersion, "version", false, "Show version")
	flag.BoolVar(&config.ShowHelp, "h", false, "Show help")
	flag.BoolVar(&config.ShowHelp, "help", false, "Show help")

	flag.Usage = printUsage
	flag.Parse()

	// Apply quiet mode
	if config.Quiet {
		config.Progress = "none"
	}

	return config
}

func runDownload(config Config, url string) int {
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

	// Create HTTP client
	httpClient := protocol.NewHTTPClient(
		protocol.WithTimeout(config.Timeout),
		protocol.WithUserAgent(fmt.Sprintf("Burkut/%s", version.Version)),
	)

	// Get file info first
	if config.Verbose {
		fmt.Fprintf(os.Stderr, "Fetching metadata from %s\n", url)
	}

	meta, err := httpClient.Head(ctx, url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get file info: %v\n", err)
		return ExitNetworkError
	}

	// Determine output path
	outputPath := determineOutputPath(config, meta.Filename)

	if config.Verbose {
		fmt.Fprintf(os.Stderr, "Filename: %s\n", meta.Filename)
		fmt.Fprintf(os.Stderr, "Size: %s\n", ui.FormatBytes(meta.ContentLength))
		fmt.Fprintf(os.Stderr, "Resume supported: %v\n", meta.AcceptRanges)
		fmt.Fprintf(os.Stderr, "Saving to: %s\n", outputPath)
	}

	// Create downloader
	downloaderConfig := engine.DefaultConfig()
	downloaderConfig.Connections = config.Connections

	downloader := engine.NewDownloader(downloaderConfig, httpClient)

	// Setup progress display
	var progressBar *ui.ProgressBar
	if config.Progress != "none" {
		progressBar = ui.NewProgressBar(
			ui.WithNoColor(config.NoColor),
			ui.WithChunks(config.Verbose),
		)

		downloader.SetProgressCallback(func(p engine.Progress) {
			switch config.Progress {
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
	if !config.Quiet && config.Progress == "bar" {
		fmt.Printf("Burkut %s - Downloading\n\n", version.Version)
	}

	// Start download
	startTime := time.Now()
	err = downloader.Download(ctx, url, outputPath)

	// Handle result
	if err != nil {
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

	// Success
	finalProgress := downloader.GetProgress()
	finalProgress.ElapsedTime = time.Since(startTime)

	if progressBar != nil && config.Progress == "bar" {
		progressBar.RenderComplete(os.Stdout, finalProgress, meta.Filename)
	} else if !config.Quiet {
		fmt.Printf("\nDownload complete: %s (%s)\n", outputPath, ui.FormatBytes(meta.ContentLength))
	}

	return ExitSuccess
}

func determineOutputPath(config Config, filename string) string {
	// Use custom output name if specified
	if config.Output != "" {
		if filepath.IsAbs(config.Output) {
			return config.Output
		}
		return filepath.Join(config.OutputDir, config.Output)
	}

	// Use filename from server
	return filepath.Join(config.OutputDir, filename)
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
  burkut --progress=minimal https://example.com/file.zip
  burkut -q https://example.com/file.zip > /dev/null

For more information, visit: https://github.com/kilimcininkoroglu/burkut
`, version.Full())
}
