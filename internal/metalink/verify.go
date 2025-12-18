// Package metalink provides verification functions for metalink piece hashes.
package metalink

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
)

// PieceVerificationResult holds the result of piece verification
type PieceVerificationResult struct {
	PieceIndex   int
	Start        int64
	End          int64
	Expected     string
	Actual       string
	Valid        bool
	Error        error
}

// VerifyFilePieces verifies all pieces of a file against metalink piece hashes
// Returns a slice of results for each piece and an overall success boolean
func (pv *PieceVerifier) VerifyFilePieces(filepath string) ([]PieceVerificationResult, bool, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, false, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	// Get file info for size check
	info, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("getting file info: %w", err)
	}

	if info.Size() != pv.totalSize {
		return nil, false, fmt.Errorf("file size mismatch: expected %d, got %d", pv.totalSize, info.Size())
	}

	pieceCount := pv.PieceCount()
	results := make([]PieceVerificationResult, pieceCount)
	allValid := true

	for i := 0; i < pieceCount; i++ {
		start, end := pv.GetPieceRange(i)
		pieceSize := end - start + 1

		// Seek to piece start
		if _, err := file.Seek(start, io.SeekStart); err != nil {
			results[i] = PieceVerificationResult{
				PieceIndex: i,
				Start:      start,
				End:        end,
				Error:      fmt.Errorf("seeking to piece: %w", err),
			}
			allValid = false
			continue
		}

		// Read piece
		pieceData := make([]byte, pieceSize)
		n, err := io.ReadFull(file, pieceData)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			results[i] = PieceVerificationResult{
				PieceIndex: i,
				Start:      start,
				End:        end,
				Error:      fmt.Errorf("reading piece: %w", err),
			}
			allValid = false
			continue
		}
		pieceData = pieceData[:n]

		// Calculate hash
		actualHash, err := calculateHash(pv.hashType, pieceData)
		if err != nil {
			results[i] = PieceVerificationResult{
				PieceIndex: i,
				Start:      start,
				End:        end,
				Error:      fmt.Errorf("calculating hash: %w", err),
			}
			allValid = false
			continue
		}

		// Compare with expected
		expectedHash, hasExpected := pv.GetExpectedHash(i)
		valid := hasExpected && actualHash == expectedHash

		results[i] = PieceVerificationResult{
			PieceIndex: i,
			Start:      start,
			End:        end,
			Expected:   expectedHash,
			Actual:     actualHash,
			Valid:      valid,
		}

		if !valid {
			allValid = false
		}
	}

	return results, allValid, nil
}

// VerifyPiece verifies a single piece of a file
func (pv *PieceVerifier) VerifyPiece(filepath string, pieceIndex int) (*PieceVerificationResult, error) {
	start, end := pv.GetPieceRange(pieceIndex)
	pieceSize := end - start + 1

	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	// Seek to piece start
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seeking to piece: %w", err)
	}

	// Read piece
	pieceData := make([]byte, pieceSize)
	n, err := io.ReadFull(file, pieceData)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("reading piece: %w", err)
	}
	pieceData = pieceData[:n]

	// Calculate hash
	actualHash, err := calculateHash(pv.hashType, pieceData)
	if err != nil {
		return nil, fmt.Errorf("calculating hash: %w", err)
	}

	// Compare with expected
	expectedHash, hasExpected := pv.GetExpectedHash(pieceIndex)
	valid := hasExpected && actualHash == expectedHash

	return &PieceVerificationResult{
		PieceIndex: pieceIndex,
		Start:      start,
		End:        end,
		Expected:   expectedHash,
		Actual:     actualHash,
		Valid:      valid,
	}, nil
}

// VerifyPieceData verifies piece data without reading from file
func (pv *PieceVerifier) VerifyPieceData(pieceIndex int, data []byte) (bool, string, error) {
	actualHash, err := calculateHash(pv.hashType, data)
	if err != nil {
		return false, "", fmt.Errorf("calculating hash: %w", err)
	}

	expectedHash, hasExpected := pv.GetExpectedHash(pieceIndex)
	if !hasExpected {
		return false, actualHash, fmt.Errorf("no expected hash for piece %d", pieceIndex)
	}

	return actualHash == expectedHash, actualHash, nil
}

// calculateHash calculates the hash of data using the specified algorithm
func calculateHash(hashType string, data []byte) (string, error) {
	var h hash.Hash

	// Normalize hash type
	normalizedType := strings.ToLower(strings.ReplaceAll(hashType, "-", ""))

	switch normalizedType {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	case "sha384":
		h = sha512.New384()
	case "sha512":
		h = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash type: %s", hashType)
	}

	h.Write(data)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GetInvalidPieces returns a list of invalid piece indices
func GetInvalidPieces(results []PieceVerificationResult) []int {
	var invalid []int
	for _, r := range results {
		if !r.Valid || r.Error != nil {
			invalid = append(invalid, r.PieceIndex)
		}
	}
	return invalid
}
