# Burkut

![Go Version](https://img.shields.io/badge/go-1.21+-blue)
![License](https://img.shields.io/badge/license-MIT-green)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS%20%7C%20FreeBSD-lightgrey)

**Burkut** is a modern download manager written in Go. Named after the golden eagle (*burkut/berkut*) in Turkish mythology — a symbol of speed, power, and precision — it combines the best of wget and curl with HTTP/3, parallel downloads, BitTorrent support, and smart resume.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
- [Examples](#examples)
- [Configuration](#config)
- [Build](#build)
- [Shell Completions](#shell-completions)
- [Exit Codes](#exit-codes)
- [Contributing](#contributing)
- [License](#license)

## Features

### Core
- **Multi-Protocol** - HTTP, HTTPS, HTTP/2, FTP, FTPS, SFTP, BitTorrent downloads
- **BitTorrent** - Magnet links and .torrent files with DHT, PEX support
- **HTTP/3 (QUIC)** - Experimental next-gen protocol support
- **Smart Resume** - Automatically resume interrupted downloads
- **Parallel Downloads** - Split files into chunks for faster downloads
- **Progress Display** - Beautiful progress bars (bar, minimal, json modes)
- **Interactive TUI** - Fullscreen mode with Bubbletea

### Advanced
- **Mirror Support** - Automatic failover to backup URLs
- **Metalink Support** - Parse .metalink/.meta4 files with multiple mirrors and piece verification
- **Recursive Download** - Spider mode for website mirroring (`-r`, `-m`)
- **Batch Downloads** - Download multiple files from URL lists
- **Checksum Verification** - MD5, SHA1, SHA256, SHA512, BLAKE3
- **Auto-verify** - Automatic checksum file detection (`--verify`)
- **Conditional Download** - Only download if newer (`-N`)
- **Certificate Pinning** - SHA256 public key pinning (`--pinnedpubkey`)
- **Spider Mode** - List URLs without downloading (`--spider`)
- **Prometheus Metrics** - Export metrics for monitoring (`--metrics-addr`)

### Configuration
- **Rate Limiting** - Global and per-host bandwidth control with wildcard support
- **Proxy Support** - HTTP and SOCKS5 proxies
- **Authentication** - Basic auth, netrc, custom headers
- **Hooks & Webhooks** - Run commands or send notifications
- **YAML Config** - Profiles for different use cases

## Installation

### Download Binary (Recommended)

Download the latest release for your platform from [GitHub Releases](https://github.com/kilimcininkoroglu/burkut/releases):

| Platform            | File                       |
|---------------------|----------------------------|
| Windows 64-bit      | `burkut-windows-amd64.exe` |
| Windows ARM         | `burkut-windows-arm64.exe` |
| Linux 64-bit        | `burkut-linux-amd64`       |
| Linux ARM64         | `burkut-linux-arm64`       |
| macOS Intel         | `burkut-darwin-amd64`      |
| macOS Apple Silicon | `burkut-darwin-arm64`      |
| FreeBSD             | `burkut-freebsd-amd64`     |

```bash
# Linux/macOS: Make executable and move to PATH
chmod +x burkut-linux-amd64
sudo mv burkut-linux-amd64 /usr/local/bin/burkut
```

### Build from Source

```bash
git clone https://github.com/kilimcininkoroglu/burkut.git
cd burkut
go build -o burkut ./cmd/burkut
```

### Go Install

```bash
go install github.com/kilimcininkoroglu/burkut/cmd/burkut@latest
```

## Quick Start

```bash
# Simple download
burkut https://example.com/file.zip

# Resume interrupted download
burkut -c https://example.com/large.iso

# 8 parallel connections
burkut -n 8 https://example.com/large.iso

# Rate limited (1 MB/s)
burkut --limit-rate 1M https://example.com/file.iso
```

## Usage

```
burkut [OPTIONS] URL

Options:
  -o, --output FILE        Output filename
  -P, --output-dir DIR     Output directory
  -c, --continue           Resume download
  -n, --connections N      Parallel connections (default: 4)
  -T, --timeout DUR        Timeout (default: 30s)
  -q, --quiet              Quiet mode
  -v, --verbose            Verbose output
  --progress TYPE          bar, minimal, json, none

Advanced:
  --limit-rate RATE        Speed limit (e.g., 10M, 500K)
  --checksum SUM           Verify checksum (sha256:..., blake3:...)
  --proxy URL              HTTP/SOCKS5 proxy
  --mirrors URLs           Fallback mirrors (comma-separated)
  -i, --input-file FILE    Batch download from file
  --on-complete CMD        Run command on success
  --webhook URL            Send webhook notification
  --http3                  Use HTTP/3 (experimental)
  --config FILE            Custom config file
  --profile NAME           Use config profile

Authentication:
  -u, --user USER:PASS     Basic authentication
  --netrc                  Use ~/.netrc for credentials
  -H, --header HEADER      Custom header (repeatable)
```

## Examples

```bash
# Checksum verification
burkut --checksum sha256:abc123... https://example.com/file.zip
burkut --checksum blake3:def456... https://example.com/file.zip

# Via proxy
burkut --proxy socks5://127.0.0.1:9050 https://example.com/file.zip

# With mirrors
burkut --mirrors "https://m1.com/f,https://m2.com/f" https://main.com/file

# Batch download
burkut -i urls.txt -P /downloads

# Webhook notification
burkut --webhook https://hooks.slack.com/xxx https://example.com/file.zip

# Authentication
burkut -u admin:secret https://example.com/protected/file.zip
burkut --netrc https://example.com/file.zip
burkut -H "Authorization: Bearer token123" https://api.example.com/download

# FTP/SFTP/FTPS
burkut ftp://ftp.example.com/pub/file.zip
burkut ftps://secure.example.com/file.zip
burkut sftp://user@host.com/path/to/file.tar.gz

# Interactive TUI mode
burkut --tui https://example.com/large-file.iso

# Metalink file (auto-selects best mirror)
burkut example.metalink

# Recursive download (website mirror)
burkut -r https://example.com/docs/
burkut -r -l 3 -A '*.pdf' https://example.com/papers/
burkut -m https://example.com/  # full site mirror

# Conditional download (only if newer)
burkut -N https://example.com/file.zip

# HTTP/2 control
burkut --http1 https://example.com/file.zip   # force HTTP/1.1
burkut --http2 https://example.com/file.zip   # force HTTP/2

# Certificate pinning
burkut --pinnedpubkey sha256//base64hash... https://secure.example.com/file

# Auto-verify checksum (fetches .sha256, .md5 automatically)
burkut --verify https://example.com/file.iso

# Spider mode (list URLs without downloading)
burkut --spider -r https://example.com/docs/

# Prometheus metrics endpoint
burkut --metrics-addr :9090 https://example.com/large-file.iso

# BitTorrent / Magnet links
burkut ubuntu-24.04.iso.torrent
burkut "magnet:?xt=urn:btih:..."
burkut --limit-rate 5M "magnet:?xt=urn:btih:..."
```

## Config

```bash
burkut --init-config  # Creates ~/.config/burkut/config.yaml
```

```yaml
general:
  connections: 4
  timeout: 30s

bandwidth:
  global_limit: "10M"
  # Per-host rate limits (supports wildcards)
  host_limits:
    - host: "slow-server.com"
      limit: "5M"
    - host: "*.cdn.example.com"
      limit: "20M"

profiles:
  fast:
    connections: 16
  tor:
    proxy:
      http: "socks5://127.0.0.1:9050"
```

## Build

### Linux / macOS

```bash
make build               # Current platform
make build-all-platforms # All platforms
make test                # Run tests
make lint                # Run linter
make check               # Full check (fmt, vet, lint, test)
```

### Windows

```cmd
build.bat build          # Current platform
build.bat build-all      # All platforms
build.bat test           # Run tests
build.bat clean          # Clean artifacts
```

## Shell Completions

```bash
# Bash
source completions/burkut.bash

# Zsh
source completions/burkut.zsh

# Fish
cp completions/burkut.fish ~/.config/fish/completions/

# PowerShell
. completions/burkut.ps1
```

## Exit Codes

| Code | Meaning              |
|------|----------------------|
| 0    | Success              |
| 1    | General error        |
| 2    | Parse/config error   |
| 3    | Network error        |
| 4    | Authentication error |
| 5    | TLS/SSL error        |
| 6    | Checksum mismatch    |
| 7    | Timeout              |
| 8    | Interrupted (Ctrl+C) |

Use in scripts:
```bash
burkut https://example.com/file.zip
if [ $? -eq 0 ]; then
    echo "Download successful"
elif [ $? -eq 6 ]; then
    echo "Checksum verification failed!"
fi
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see [LICENSE](LICENSE) file for details.
