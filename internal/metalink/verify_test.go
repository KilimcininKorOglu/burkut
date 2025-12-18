package metalink

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestPieceVerifier(t *testing.T) {
	// Create test file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")

	// Create 100 bytes of test data (will be split into pieces)
	testData := make([]byte, 100)
	for i := range testData {
		testData[i] = byte(i)
	}

	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Calculate expected hashes for 25-byte pieces
	pieceLength := int64(25)
	var pieceHashes []PieceHash

	for i := 0; i < 4; i++ {
		start := i * int(pieceLength)
		end := start + int(pieceLength)
		if end > len(testData) {
			end = len(testData)
		}

		h := sha256.Sum256(testData[start:end])
		pieceHashes = append(pieceHashes, PieceHash{
			Piece: i,
			Value: hex.EncodeToString(h[:]),
		})
	}

	// Create File struct
	file := &File{
		Name: "test.bin",
		Size: 100,
		Pieces: &Pieces{
			Type:   "sha-256",
			Length: pieceLength,
			Hashes: pieceHashes,
		},
	}

	// Test HasPieceHashes
	if !file.HasPieceHashes() {
		t.Error("HasPieceHashes() should return true")
	}

	// Create verifier
	pv := NewPieceVerifier(file)
	if pv == nil {
		t.Fatal("NewPieceVerifier() returned nil")
	}

	// Test PieceCount
	if pv.PieceCount() != 4 {
		t.Errorf("PieceCount() = %d, want 4", pv.PieceCount())
	}

	// Test HashType
	if pv.HashType() != "sha-256" {
		t.Errorf("HashType() = %s, want sha-256", pv.HashType())
	}

	// Test GetPieceRange
	start, end := pv.GetPieceRange(0)
	if start != 0 || end != 24 {
		t.Errorf("GetPieceRange(0) = (%d, %d), want (0, 24)", start, end)
	}

	start, end = pv.GetPieceRange(3)
	if start != 75 || end != 99 {
		t.Errorf("GetPieceRange(3) = (%d, %d), want (75, 99)", start, end)
	}

	// Test VerifyFilePieces
	results, allValid, err := pv.VerifyFilePieces(testFile)
	if err != nil {
		t.Fatalf("VerifyFilePieces() error = %v", err)
	}

	if !allValid {
		t.Error("VerifyFilePieces() allValid = false, want true")
		for _, r := range results {
			if !r.Valid {
				t.Errorf("Piece %d: expected %s, got %s", r.PieceIndex, r.Expected, r.Actual)
			}
		}
	}

	if len(results) != 4 {
		t.Errorf("VerifyFilePieces() returned %d results, want 4", len(results))
	}

	// Test VerifyPiece
	result, err := pv.VerifyPiece(testFile, 0)
	if err != nil {
		t.Fatalf("VerifyPiece() error = %v", err)
	}
	if !result.Valid {
		t.Error("VerifyPiece(0) should be valid")
	}

	// Test VerifyPieceData
	valid, _, err := pv.VerifyPieceData(0, testData[0:25])
	if err != nil {
		t.Fatalf("VerifyPieceData() error = %v", err)
	}
	if !valid {
		t.Error("VerifyPieceData() should return true for correct data")
	}

	// Test with wrong data
	wrongData := make([]byte, 25)
	valid, _, err = pv.VerifyPieceData(0, wrongData)
	if err != nil {
		t.Fatalf("VerifyPieceData() error = %v", err)
	}
	if valid {
		t.Error("VerifyPieceData() should return false for wrong data")
	}
}

func TestPieceVerifier_NoPieces(t *testing.T) {
	file := &File{
		Name: "test.bin",
		Size: 100,
	}

	if file.HasPieceHashes() {
		t.Error("HasPieceHashes() should return false for file without pieces")
	}

	pv := NewPieceVerifier(file)
	if pv != nil {
		t.Error("NewPieceVerifier() should return nil for file without pieces")
	}
}

func TestGetInvalidPieces(t *testing.T) {
	results := []PieceVerificationResult{
		{PieceIndex: 0, Valid: true},
		{PieceIndex: 1, Valid: false},
		{PieceIndex: 2, Valid: true},
		{PieceIndex: 3, Valid: false},
	}

	invalid := GetInvalidPieces(results)
	if len(invalid) != 2 {
		t.Errorf("GetInvalidPieces() returned %d items, want 2", len(invalid))
	}

	if invalid[0] != 1 || invalid[1] != 3 {
		t.Errorf("GetInvalidPieces() = %v, want [1, 3]", invalid)
	}
}

func TestCalculateHash(t *testing.T) {
	data := []byte("test data")

	tests := []struct {
		hashType string
		wantLen  int // hex string length
	}{
		{"md5", 32},
		{"sha1", 40},
		{"sha256", 64},
		{"sha-256", 64}, // with dash
		{"sha512", 128},
		{"sha384", 96},
	}

	for _, tt := range tests {
		t.Run(tt.hashType, func(t *testing.T) {
			hash, err := calculateHash(tt.hashType, data)
			if err != nil {
				t.Errorf("calculateHash(%q) error = %v", tt.hashType, err)
				return
			}
			if len(hash) != tt.wantLen {
				t.Errorf("calculateHash(%q) returned hash of length %d, want %d", tt.hashType, len(hash), tt.wantLen)
			}
		})
	}

	// Test unsupported algorithm
	_, err := calculateHash("unknown", data)
	if err == nil {
		t.Error("calculateHash(unknown) should return error")
	}
}
