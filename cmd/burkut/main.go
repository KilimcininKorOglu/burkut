// Burkut - Modern Download Manager
// Named after the golden eagle (berkut) in Turkish mythology
package main

import (
	"fmt"
	"os"

	"github.com/kilimcininkoroglu/burkut/internal/version"
)

func main() {
	// For now, just print version info
	// CLI framework will be added in later commits
	if len(os.Args) > 1 && (os.Args[1] == "-V" || os.Args[1] == "--version") {
		fmt.Println(version.Full())
		os.Exit(0)
	}

	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		printUsage()
		os.Exit(0)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// URL argument
	url := os.Args[1]
	fmt.Printf("Burkut %s\n", version.Version)
	fmt.Printf("URL: %s\n", url)
	fmt.Println("Download functionality will be implemented in upcoming commits.")
}

func printUsage() {
	fmt.Printf(`%s

Usage:
  burkut [OPTIONS] URL [URL...]
  burkut -i FILE

A modern download manager combining the best of wget and curl
with HTTP/2-3 support, parallel downloads, and smart resume.

Options:
  -o, --output FILE      Write output to FILE
  -P, --output-dir DIR   Save files to DIR
  -c, --continue         Resume download
  -n, --connections N    Number of parallel connections (default: 4)
  -q, --quiet            Quiet mode
  -v, --verbose          Verbose output
  -h, --help             Show this help
  -V, --version          Show version

Examples:
  burkut https://example.com/file.zip
  burkut -o myfile.zip https://example.com/file.zip
  burkut -c https://example.com/large-file.iso

For more information, visit: https://github.com/kilimcininkoroglu/burkut
`, version.Full())
}
