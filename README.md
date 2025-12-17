# 🦅 Burkut

**Burkut** is a modern download manager written in Go. Named after the golden eagle (*berkut*) in Turkish mythology, it combines the best of wget and curl with HTTP/3, parallel downloads, and smart resume.

## ✨ Features

- 📥 **HTTP/HTTPS Downloads** with parallel chunks
- ⚡ **HTTP/3 (QUIC)** experimental support
- 🔄 **Smart Resume** for interrupted downloads
- 📊 **Progress Display** (bar, minimal, json modes)
- 🔀 **Mirror Support** with automatic failover
- 📦 **Batch Downloads** from URL list files
- 🔐 **Checksum Verification** (MD5, SHA1, SHA256, SHA512)
- ⏱️ **Rate Limiting** for bandwidth control
- 🌍 **Proxy Support** (HTTP, SOCKS5)
- 🪝 **Hooks & Webhooks** for notifications
- ⚙️ **YAML Config** with profiles

## 📦 Installation

```bash
git clone https://github.com/kilimcininkoroglu/burkut.git
cd burkut
go build -o burkut ./cmd/burkut
```

## 🚀 Quick Start

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

## 📖 Usage

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
  --checksum SUM           Verify checksum
  --proxy URL              HTTP/SOCKS5 proxy
  --mirrors URLs           Fallback mirrors (comma-separated)
  -i, --input-file FILE    Batch download from file
  --on-complete CMD        Run command on success
  --webhook URL            Send webhook notification
  --http3                  Use HTTP/3 (experimental)
  --config FILE            Custom config file
  --profile NAME           Use config profile
```

## 📋 Examples

```bash
# Checksum verification
burkut --checksum sha256:abc123... https://example.com/file.zip

# Via SOCKS5 proxy
burkut --proxy socks5://127.0.0.1:9050 https://example.com/file.zip

# With mirrors
burkut --mirrors "https://m1.com/f,https://m2.com/f" https://main.com/file

# Batch download
burkut -i urls.txt -P /downloads

# Webhook notification
burkut --webhook https://hooks.slack.com/xxx https://example.com/file.zip
```

## ⚙️ Config

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

## 🔧 Build

```bash
make build              # Current platform
make build-all-platforms # All platforms
make test               # Run tests
```

## 📄 License

MIT License
