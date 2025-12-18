package metalink

import (
	"strings"
	"testing"
)

func TestParse_Metalink4(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<metalink xmlns="urn:ietf:params:xml:ns:metalink">
  <file name="example.iso">
    <size>731279360</size>
    <description>Example ISO image</description>
    <hash type="sha-256">e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855</hash>
    <hash type="md5">d41d8cd98f00b204e9800998ecf8427e</hash>
    <url priority="1" location="us">https://mirror1.example.com/example.iso</url>
    <url priority="2" location="de">https://mirror2.example.com/example.iso</url>
    <url priority="3" location="jp">https://mirror3.example.com/example.iso</url>
  </file>
</metalink>`

	ml, err := Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(ml.Files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(ml.Files))
	}

	file := ml.Files[0]

	// Check file name
	if file.Name != "example.iso" {
		t.Errorf("Name = %q, want %q", file.Name, "example.iso")
	}

	// Check size
	if file.Size != 731279360 {
		t.Errorf("Size = %d, want %d", file.Size, 731279360)
	}

	// Check hashes
	if len(file.Hashes) != 2 {
		t.Errorf("Hashes count = %d, want 2", len(file.Hashes))
	}

	sha256 := file.GetChecksum("sha-256")
	if sha256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("SHA-256 = %q", sha256)
	}

	// Check URLs
	if len(file.URLs) != 3 {
		t.Errorf("URLs count = %d, want 3", len(file.URLs))
	}

	// Check sorted URLs
	sorted := file.SortedURLs()
	if sorted[0].Priority != 1 {
		t.Errorf("First URL priority = %d, want 1", sorted[0].Priority)
	}
	if sorted[0].Location != "us" {
		t.Errorf("First URL location = %q, want %q", sorted[0].Location, "us")
	}
}

func TestParse_Metalink3(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<metalink version="3.0">
  <files>
    <file name="example.zip">
      <size>1048576</size>
      <verification>
        <hash type="md5">098f6bcd4621d373cade4e832627b4f6</hash>
        <hash type="sha1">a94a8fe5ccb19ba61c4c0873d391e987982fbbd3</hash>
      </verification>
      <resources>
        <url type="http" preference="100">https://primary.example.com/example.zip</url>
        <url type="http" preference="50" location="eu">https://backup.example.com/example.zip</url>
      </resources>
    </file>
  </files>
</metalink>`

	ml, err := Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(ml.Files) != 1 {
		t.Fatalf("Expected 1 file (converted from V3), got %d", len(ml.Files))
	}

	file := ml.Files[0]

	if file.Name != "example.zip" {
		t.Errorf("Name = %q, want %q", file.Name, "example.zip")
	}

	if file.Size != 1048576 {
		t.Errorf("Size = %d, want %d", file.Size, 1048576)
	}

	// Check converted hashes
	if len(file.Hashes) != 2 {
		t.Errorf("Hashes count = %d, want 2", len(file.Hashes))
	}

	// Check converted URLs
	if len(file.URLs) != 2 {
		t.Errorf("URLs count = %d, want 2", len(file.URLs))
	}

	// Check priority conversion (preference 100 -> priority 1)
	sorted := file.SortedURLs()
	if sorted[0].URL != "https://primary.example.com/example.zip" {
		t.Errorf("Primary URL not first in sorted list")
	}
}

func TestFile_GetPreferredChecksum(t *testing.T) {
	file := File{
		Hashes: []Hash{
			{Type: "md5", Value: "md5hash"},
			{Type: "sha-256", Value: "sha256hash"},
			{Type: "sha-1", Value: "sha1hash"},
		},
	}

	hashType, value := file.GetPreferredChecksum()
	if hashType != "sha-256" {
		t.Errorf("Preferred hash type = %q, want %q", hashType, "sha-256")
	}
	if value != "sha256hash" {
		t.Errorf("Preferred hash value = %q, want %q", value, "sha256hash")
	}
}

func TestFile_GetChecksum_CaseInsensitive(t *testing.T) {
	file := File{
		Hashes: []Hash{
			{Type: "SHA-256", Value: "hash1"},
			{Type: "MD5", Value: "hash2"},
		},
	}

	if file.GetChecksum("sha-256") != "hash1" {
		t.Error("Case-insensitive lookup failed for SHA-256")
	}
	if file.GetChecksum("md5") != "hash2" {
		t.Error("Case-insensitive lookup failed for MD5")
	}
}

func TestMetalink_GetFile(t *testing.T) {
	ml := Metalink{
		Files: []File{
			{Name: "file1.zip"},
			{Name: "file2.zip"},
		},
	}

	file := ml.GetFile()
	if file == nil {
		t.Fatal("GetFile returned nil")
	}
	if file.Name != "file1.zip" {
		t.Errorf("GetFile returned %q, want %q", file.Name, "file1.zip")
	}
}

func TestMetalink_GetFileByName(t *testing.T) {
	ml := Metalink{
		Files: []File{
			{Name: "file1.zip"},
			{Name: "file2.zip"},
		},
	}

	file := ml.GetFileByName("file2.zip")
	if file == nil {
		t.Fatal("GetFileByName returned nil")
	}
	if file.Name != "file2.zip" {
		t.Errorf("GetFileByName returned %q, want %q", file.Name, "file2.zip")
	}

	// Non-existent file
	if ml.GetFileByName("nonexistent.zip") != nil {
		t.Error("GetFileByName should return nil for non-existent file")
	}
}

func TestIsMetalink(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"file.metalink", true},
		{"file.meta4", true},
		{"FILE.METALINK", true},
		{"FILE.META4", true},
		{"file.xml", false},
		{"file.zip", false},
		{"metalink.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if IsMetalink(tt.filename) != tt.expected {
				t.Errorf("IsMetalink(%q) = %v, want %v", tt.filename, !tt.expected, tt.expected)
			}
		})
	}
}

func TestParse_WithPieces(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<metalink xmlns="urn:ietf:params:xml:ns:metalink">
  <file name="large.iso">
    <size>4294967296</size>
    <hash type="sha-256">abcd1234</hash>
    <pieces type="sha-256" length="1048576">
      <hash piece="0">piece0hash</hash>
      <hash piece="1">piece1hash</hash>
      <hash piece="2">piece2hash</hash>
    </pieces>
    <url>https://example.com/large.iso</url>
  </file>
</metalink>`

	ml, err := Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	file := ml.GetFile()
	if file.Pieces == nil {
		t.Fatal("Pieces should not be nil")
	}

	if file.Pieces.Type != "sha-256" {
		t.Errorf("Pieces type = %q, want %q", file.Pieces.Type, "sha-256")
	}

	if file.Pieces.Length != 1048576 {
		t.Errorf("Pieces length = %d, want %d", file.Pieces.Length, 1048576)
	}

	if len(file.Pieces.Hashes) != 3 {
		t.Errorf("Piece hashes count = %d, want 3", len(file.Pieces.Hashes))
	}
}
