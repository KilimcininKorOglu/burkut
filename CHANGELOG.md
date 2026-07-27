# Changelog

All notable changes to Burkut will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [v0.2.2] - 2026-07-27

### Added
- Add MIT LICENSE file so release archives build; goreleaser required it and its absence had failed every prior release

## [v0.2.1] - 2026-07-27

### Security
- Bump Go floor to 1.25.12 to clear 22 called standard-library CVEs in crypto/x509, crypto/tls, net/url, net/http, net/textproto, net, encoding/asn1 and encoding/pem (bcf8d82)
- Upgrade golang.org/x/net, x/crypto, x/text and quic-go to their patched releases (bcf8d82)
- Track the Go floor via `go-version-file: go.mod` in the release workflow so CI cannot drift from it (bcf8d82)

### Changed
- Clear all linter findings (errcheck, staticcheck, modernize) and guard ftp GetRange against a negative offset (8d049f3)
- Annotate every by-design gosec finding with a line-scoped `#nosec` and justification; no rule is disabled globally (8d049f3)

## [v0.2.0] - 2026-03-15

### Security
- Reject SFTP connections when host key verification is not available (88f039f)
- Sanitize environment variables in hook command execution (cd7f9bd)
- Validate webhook URLs to prevent SSRF (5a64444)

### Bug Fixes
- Use write lock in GetProgress to prevent race condition (ef3849f)
- Use runtime.GOOS instead of OS environment variable for platform detection (db642e3)
- Add read/write timeouts to metrics HTTP server (5a8644e)
- Add validation for conflicting and invalid CLI flag values (3e68b49)
- Use strconv.ParseInt for Content-Length in HTTP/3 client (d9f54b9)
- Prevent array index out of bounds in FormatBytes (0c92f3b)
- Reduce busy-wait in crawler work feeder with consecutive empty checks (2c684df)
- Accept parent context in QueueManager constructor (42b327c)
- Add panic recovery in async TUI goroutine (d3a90c5)
- Add panic recovery and error logging in async hook execution (5edfbcb)
- Add IPv6 loopback to default NoProxy list (ba2f437)
- Warn when SSH private key file has overly permissive permissions (5c22592)
- Add missing CLI flags to bash and zsh completion scripts (8fb5906)
- Handle ANSI color fallback in build.bat for older Windows (3cbfbe1)

### Documentation
- Sync README usage section with actual --help output (57dce78)

## [v0.1.1] - 2026-03-15

### Refactoring
- Replace hardcoded user-agent versions with dynamic values (bb8a9d9)

## [v0.1.0] - 2026-03-15

### Features
- Add BitTorrent and magnet link support (77669d5)
- Add use_tui option for default TUI mode (b75bc98)
- Auto-create default config file on first run (26e3d3a)
- Add Prometheus metrics endpoint (1889f67)
- Add implicit FTPS support for port 990 (36b5dbd)
- Add piece hash verification for Metalink (eb569d7)
- Add auto-verify checksum file detection (a6a4ccb)
- Add --spider mode for URL listing without download (c89f5af)
- Implement convert-links (-k) feature for crawler (80c6f60)
- Add FTP/FTPS CLI integration (d11e7fb)
- Add Metalink file support (febc421)
- Add Metalink 3/4 parser (33c3f73)
- Add FTPS support - FTP over TLS (05eaf0a)
- Add recursive download / spider mode CLI integration (467c4c9)
- Add recursive website download package (2a4f817)
- Add certificate pinning with --pinnedpubkey flag (2626558)
- Add conditional download with --timestamping (-N) flag (b626bde)
- Add HTTP/2 explicit control with --http1 and --http2 flags (af72c8e)
- Add interactive TUI mode using Bubbletea (499f609)
- Add shell completions for bash, zsh, fish, and PowerShell (e5c1689)
- Add BLAKE3, netrc, headers, FTP, and SFTP support (fd91c6c)
- Add HTTP/3 (QUIC) protocol support (e8d4abd)
- Add mirror/fallback URL support (8bdda1a)
- Add event hooks and webhook notifications (051dc6c)
- Add batch download from URL file (38f0b39)
- Add proxy support and retry mechanism (caf3f99)
- Add config, checksum, and rate limiting support (aa84d84)
- Add per-host rate limiting with wildcard support (af0e007)
- Improve Content-Disposition parsing and security (4b28458)
- Implement full CLI with download functionality (4e78ba3)
- Add progress bar and display components (62edbbf)
- Add parallel chunk downloader with resume support (e731145)
- Add state persistence for resume support (0fbe0bd)
- Add FileWriter for disk I/O operations (0728bf3)
- Add HTTP client with HEAD, GET, and Range support (c32e50b)
- Initialize project structure (d0c2e1d)

### Bug Fixes
- Update goreleaser config and align build scripts (4b0b997)

### Documentation
- Update README with correct Go version, build command, and Linux ARM platform (8ac3ad2)
- Fix table alignments in README (95dfb60)
- Improve README with badges, TOC, installation options (a90468c)
- Update README with new features and examples (8b4b355, c287fdd)
- Add README, examples, and fix build scripts (7a39c19)
- Remove redundant emojis from README (c4a1fb9)
- Add TUI mode to README (5d8d06e)
