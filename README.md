# Burkut

**Burkut** is a modern download manager written in Go. Named after the golden eagle (*berkut*) in Turkish mythology, it combines the best of wget and curl with HTTP/3, parallel downloads, and smart resume.

## Features

- **Multi-Protocol** - HTTP, HTTPS, FTP, SFTP downloads
- **HTTP/3 (QUIC)** - Experimental next-gen protocol support
- **Smart Resume** - Automatically resume interrupted downloads
- **Parallel Downloads** - Split files into chunks for faster downloads
- **Progress Display** - Beautiful progress bars (bar, minimal, json modes)
- **Mirror Support** - Automatic failover to backup URLs
- **Batch Downloads** - Download multiple files from URL lists
- **Checksum Verification** - MD5, SHA1, SHA256, SHA512, BLAKE3
- **Rate Limiting** - Control bandwidth usage
- **Proxy Support** - HTTP and SOCKS5 proxies
- **Authentication** - Basic auth, netrc, custom headers
- **Hooks & Webhooks** - Run commands or send notifications
- **YAML Config** - Profiles for different use cases
- **Interactive TUI** - Fullscreen mode with Bubbletea

## Installation

```bash
git clone https://github.com/kilimcininkoroglu/burkut.git
cd burkut
go build -o burkut ./cmd/burkut
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

# FTP/SFTP
burkut ftp://ftp.example.com/pub/file.zip
burkut sftp://user@host.com/path/to/file.tar.gz

# Interactive TUI mode
burkut --tui https://example.com/large-file.iso
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

profiles:
  fast:
    connections: 16
  tor:
    proxy:
      http: "socks5://127.0.0.1:9050"
```

## Build

```bash
make build              # Current platform
make build-all-platforms # All platforms
make test               # Run tests
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

## License

MIT License
