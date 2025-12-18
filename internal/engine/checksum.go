package engine

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"

	"github.com/zeebo/blake3"
)

// ChecksumAlgorithm represents a supported checksum algorithm
type ChecksumAlgorithm string

const (
	AlgorithmMD5    ChecksumAlgorithm = "md5"
	AlgorithmSHA1   ChecksumAlgorithm = "sha1"
	AlgorithmSHA256 ChecksumAlgorithm = "sha256"
	AlgorithmSHA512 ChecksumAlgorithm = "sha512"
	AlgorithmBLAKE3 ChecksumAlgorithm = "blake3"
)

// Checksum represents a checksum value with its algorithm
type Checksum struct {
	Algorithm ChecksumAlgorithm
	Value     string
}

// ParseChecksum parses a checksum string in format "algorithm:value"
// Examples: "sha256:abc123...", "md5:def456..."
func ParseChecksum(s string) (*Checksum, error) {
	if s == "" {
		return nil, nil
	}

	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid checksum format, expected 'algorithm:value'")
	}

	algorithm := ChecksumAlgorithm(strings.ToLower(parts[0]))
	value := strings.ToLower(parts[1])

	// Validate algorithm
	switch algorithm {
	case AlgorithmMD5, AlgorithmSHA1, AlgorithmSHA256, AlgorithmSHA512, AlgorithmBLAKE3:
		// Valid
	default:
		return nil, fmt.Errorf("unsupported checksum algorithm: %s (supported: md5, sha1, sha256, sha512, blake3)", algorithm)
	}

	// Validate hex value
	if _, err := hex.DecodeString(value); err != nil {
		return nil, fmt.Errorf("invalid checksum hex value: %w", err)
	}

	return &Checksum{
		Algorithm: algorithm,
		Value:     value,
	}, nil
}

// String returns the checksum in "algorithm:value" format
func (c *Checksum) String() string {
	return fmt.Sprintf("%s:%s", c.Algorithm, c.Value)
}

// newHasher creates a new hash.Hash for the given algorithm
func newHasher(algorithm ChecksumAlgorithm) (hash.Hash, error) {
	switch algorithm {
	case AlgorithmMD5:
		return md5.New(), nil
	case AlgorithmSHA1:
		return sha1.New(), nil
	case AlgorithmSHA256:
		return sha256.New(), nil
	case AlgorithmSHA512:
		return sha512.New(), nil
	case AlgorithmBLAKE3:
		return blake3.New(), nil
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
}

// CalculateChecksum calculates the checksum of a file
func CalculateChecksum(filepath string, algorithm ChecksumAlgorithm) (*Checksum, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	hasher, err := newHasher(algorithm)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(hasher, file); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return &Checksum{
		Algorithm: algorithm,
		Value:     hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

// CalculateChecksumReader calculates checksum from a reader
func CalculateChecksumReader(r io.Reader, algorithm ChecksumAlgorithm) (*Checksum, error) {
	hasher, err := newHasher(algorithm)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(hasher, r); err != nil {
		return nil, fmt.Errorf("reading data: %w", err)
	}

	return &Checksum{
		Algorithm: algorithm,
		Value:     hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

// VerifyChecksum verifies a file against an expected checksum
func VerifyChecksum(filepath string, expected *Checksum) (bool, error) {
	if expected == nil {
		return true, nil // No checksum to verify
	}

	actual, err := CalculateChecksum(filepath, expected.Algorithm)
	if err != nil {
		return false, err
	}

	return actual.Value == expected.Value, nil
}

// ChecksumWriter wraps an io.Writer and calculates checksum while writing
type ChecksumWriter struct {
	writer io.Writer
	hasher hash.Hash
	algorithm ChecksumAlgorithm
}

// NewChecksumWriter creates a writer that calculates checksum while writing
func NewChecksumWriter(w io.Writer, algorithm ChecksumAlgorithm) (*ChecksumWriter, error) {
	hasher, err := newHasher(algorithm)
	if err != nil {
		return nil, err
	}

	return &ChecksumWriter{
		writer:    w,
		hasher:    hasher,
		algorithm: algorithm,
	}, nil
}

// Write writes data to the underlying writer and updates the checksum
func (cw *ChecksumWriter) Write(p []byte) (int, error) {
	n, err := cw.writer.Write(p)
	if n > 0 {
		cw.hasher.Write(p[:n])
	}
	return n, err
}

// Checksum returns the calculated checksum
func (cw *ChecksumWriter) Checksum() *Checksum {
	return &Checksum{
		Algorithm: cw.algorithm,
		Value:     hex.EncodeToString(cw.hasher.Sum(nil)),
	}
}

// Reset resets the checksum calculation
func (cw *ChecksumWriter) Reset() {
	cw.hasher.Reset()
}

// DetectAlgorithmFromLength tries to detect the algorithm from checksum length
func DetectAlgorithmFromLength(checksumValue string) ChecksumAlgorithm {
	switch len(checksumValue) {
	case 32: // MD5 produces 128-bit (16 bytes = 32 hex chars)
		return AlgorithmMD5
	case 40: // SHA1 produces 160-bit (20 bytes = 40 hex chars)
		return AlgorithmSHA1
	case 64: // SHA256 or BLAKE3 produces 256-bit (32 bytes = 64 hex chars)
		// Default to SHA256, user can specify blake3: prefix if needed
		return AlgorithmSHA256
	case 128: // SHA512 produces 512-bit (64 bytes = 128 hex chars)
		return AlgorithmSHA512
	default:
		return AlgorithmSHA256 // Default assumption
	}
}

// ParseChecksumAuto parses a checksum value and auto-detects the algorithm
func ParseChecksumAuto(value string) (*Checksum, error) {
	value = strings.TrimSpace(strings.ToLower(value))

	// Check if it has algorithm prefix
	if strings.Contains(value, ":") {
		return ParseChecksum(value)
	}

	// Auto-detect from length
	algorithm := DetectAlgorithmFromLength(value)

	// Validate hex
	if _, err := hex.DecodeString(value); err != nil {
		return nil, fmt.Errorf("invalid checksum hex value: %w", err)
	}

	return &Checksum{
		Algorithm: algorithm,
		Value:     value,
	}, nil
}

// ChecksumFileExtensions maps file extensions to their checksum algorithms
var ChecksumFileExtensions = map[string]ChecksumAlgorithm{
	".sha256":    AlgorithmSHA256,
	".sha256sum": AlgorithmSHA256,
	".sha512":    AlgorithmSHA512,
	".sha512sum": AlgorithmSHA512,
	".sha1":      AlgorithmSHA1,
	".sha1sum":   AlgorithmSHA1,
	".md5":       AlgorithmMD5,
	".md5sum":    AlgorithmMD5,
}

// FindChecksumFile looks for a checksum file for the given filename
// It tries common extensions like .sha256, .md5, SHA256SUMS, etc.
// Returns the checksum file path and algorithm if found
func FindChecksumFile(filepath string) (checksumFile string, algorithm ChecksumAlgorithm, found bool) {
	// Try direct extensions first
	for ext, alg := range ChecksumFileExtensions {
		candidate := filepath + ext
		if _, err := os.Stat(candidate); err == nil {
			return candidate, alg, true
		}
	}

	return "", "", false
}

// ParseChecksumFile parses a checksum file and extracts the checksum for the given filename
// Supports formats:
//   - "checksum  filename" (GNU coreutils format)
//   - "checksum *filename" (binary mode)
//   - "checksum filename" (simple format)
//   - Just "checksum" (single checksum for single file)
func ParseChecksumFile(checksumFilePath string, targetFilename string) (string, error) {
	data, err := os.ReadFile(checksumFilePath)
	if err != nil {
		return "", fmt.Errorf("reading checksum file: %w", err)
	}

	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Try to parse "checksum  filename" or "checksum *filename" format
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksum := parts[0]
			filename := strings.TrimPrefix(parts[len(parts)-1], "*")

			// Check if this line is for our file
			if filename == targetFilename || strings.HasSuffix(filename, "/"+targetFilename) {
				return checksum, nil
			}
		} else if len(parts) == 1 {
			// Single checksum (no filename) - assume it's for the target file
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("checksum for %q not found in %s", targetFilename, checksumFilePath)
}

// FetchAndParseChecksumURL fetches a checksum file from URL and extracts checksum
// Returns the checksum string (without algorithm prefix)
func FetchAndParseChecksumURL(checksumURL string, targetFilename string, httpGet func(url string) ([]byte, error)) (string, ChecksumAlgorithm, error) {
	// Determine algorithm from URL extension
	algorithm := AlgorithmSHA256 // Default
	urlLower := strings.ToLower(checksumURL)
	for ext, alg := range ChecksumFileExtensions {
		if strings.HasSuffix(urlLower, ext) {
			algorithm = alg
			break
		}
	}

	// Also check for SUMS file naming convention
	if strings.Contains(urlLower, "sha256sum") || strings.Contains(urlLower, "sha256") {
		algorithm = AlgorithmSHA256
	} else if strings.Contains(urlLower, "sha512sum") || strings.Contains(urlLower, "sha512") {
		algorithm = AlgorithmSHA512
	} else if strings.Contains(urlLower, "md5sum") || strings.Contains(urlLower, "md5") {
		algorithm = AlgorithmMD5
	} else if strings.Contains(urlLower, "sha1sum") || strings.Contains(urlLower, "sha1") {
		algorithm = AlgorithmSHA1
	}

	// Fetch the checksum file
	data, err := httpGet(checksumURL)
	if err != nil {
		return "", "", fmt.Errorf("fetching checksum file: %w", err)
	}

	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksum := parts[0]
			filename := strings.TrimPrefix(parts[len(parts)-1], "*")

			if filename == targetFilename || strings.HasSuffix(filename, "/"+targetFilename) {
				return checksum, algorithm, nil
			}
		} else if len(parts) == 1 {
			return parts[0], algorithm, nil
		}
	}

	return "", "", fmt.Errorf("checksum for %q not found", targetFilename)
}
