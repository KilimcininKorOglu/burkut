// Package metalink provides parsing and handling of Metalink files (.metalink, .meta4).
// Metalink is an XML format that describes download sources, checksums, and file pieces.
package metalink

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// Metalink represents a Metalink 4 (RFC 5854) or Metalink 3 document
type Metalink struct {
	XMLName xml.Name `xml:"metalink"`
	Version string   `xml:"version,attr"` // Metalink 3 only
	XMLNS   string   `xml:"xmlns,attr"`   // Metalink 4: urn:ietf:params:xml:ns:metalink
	Files   []File   `xml:"file"`
	// Metalink 3 format
	FilesV3 []FileV3 `xml:"files>file"`
}

// File represents a file entry in Metalink 4 format
type File struct {
	Name        string   `xml:"name,attr"`
	Size        int64    `xml:"size"`
	Description string   `xml:"description,omitempty"`
	Publisher   *Publisher `xml:"publisher,omitempty"`
	Hashes      []Hash   `xml:"hash"`
	Pieces      *Pieces  `xml:"pieces,omitempty"`
	URLs        []URL    `xml:"url"`
	MetaURLs    []MetaURL `xml:"metaurl,omitempty"`
}

// FileV3 represents a file entry in Metalink 3 format
type FileV3 struct {
	Name        string   `xml:"name,attr"`
	Size        int64    `xml:"size"`
	Description string   `xml:"description,omitempty"`
	Publisher   *PublisherV3 `xml:"publisher,omitempty"`
	Verification *VerificationV3 `xml:"verification,omitempty"`
	Resources   []ResourceV3 `xml:"resources>url"`
}

// Publisher represents file publisher info
type Publisher struct {
	Name string `xml:"name"`
	URL  string `xml:"url,omitempty"`
}

// PublisherV3 represents publisher in Metalink 3 format
type PublisherV3 struct {
	Name string `xml:"name,attr"`
	URL  string `xml:"url,attr,omitempty"`
}

// Hash represents a file hash
type Hash struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// VerificationV3 represents verification info in Metalink 3
type VerificationV3 struct {
	Hashes []HashV3 `xml:"hash"`
	Pieces *PiecesV3 `xml:"pieces,omitempty"`
}

// HashV3 represents a hash in Metalink 3 format
type HashV3 struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// Pieces represents piece hashes for segmented download verification
type Pieces struct {
	Type   string  `xml:"type,attr"`
	Length int64   `xml:"length,attr"`
	Hashes []PieceHash `xml:"hash"`
}

// PiecesV3 represents pieces in Metalink 3 format
type PiecesV3 struct {
	Type   string  `xml:"type,attr"`
	Length int64   `xml:"length,attr"`
	Hashes []PieceHashV3 `xml:"hash"`
}

// PieceHash represents a single piece hash
type PieceHash struct {
	Piece int    `xml:"piece,attr"`
	Value string `xml:",chardata"`
}

// PieceHashV3 represents a piece hash in Metalink 3 format
type PieceHashV3 struct {
	Piece int    `xml:"piece,attr"`
	Value string `xml:",chardata"`
}

// URL represents a download URL
type URL struct {
	Priority int    `xml:"priority,attr,omitempty"`
	Location string `xml:"location,attr,omitempty"`
	URL      string `xml:",chardata"`
}

// MetaURL represents a metaurl (torrent, magnet, etc.)
type MetaURL struct {
	Priority  int    `xml:"priority,attr,omitempty"`
	MediaType string `xml:"mediatype,attr"`
	Name      string `xml:"name,attr,omitempty"`
	URL       string `xml:",chardata"`
}

// ResourceV3 represents a resource URL in Metalink 3 format
type ResourceV3 struct {
	Type       string `xml:"type,attr"`
	Location   string `xml:"location,attr,omitempty"`
	Preference int    `xml:"preference,attr,omitempty"`
	URL        string `xml:",chardata"`
}

// ParseFile parses a Metalink file from disk
func ParseFile(filename string) (*Metalink, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("opening metalink file: %w", err)
	}
	defer f.Close()

	return Parse(f)
}

// Parse parses a Metalink document from an io.Reader
func Parse(r io.Reader) (*Metalink, error) {
	var ml Metalink
	decoder := xml.NewDecoder(r)
	if err := decoder.Decode(&ml); err != nil {
		return nil, fmt.Errorf("parsing metalink XML: %w", err)
	}

	// Convert Metalink 3 format to unified format
	if len(ml.FilesV3) > 0 && len(ml.Files) == 0 {
		ml.Files = convertV3ToV4(ml.FilesV3)
	}

	return &ml, nil
}

// convertV3ToV4 converts Metalink 3 files to Metalink 4 format
func convertV3ToV4(filesV3 []FileV3) []File {
	files := make([]File, len(filesV3))
	for i, f3 := range filesV3 {
		file := File{
			Name:        f3.Name,
			Size:        f3.Size,
			Description: f3.Description,
		}

		// Convert publisher
		if f3.Publisher != nil {
			file.Publisher = &Publisher{
				Name: f3.Publisher.Name,
				URL:  f3.Publisher.URL,
			}
		}

		// Convert hashes
		if f3.Verification != nil {
			for _, h := range f3.Verification.Hashes {
				file.Hashes = append(file.Hashes, Hash{
					Type:  h.Type,
					Value: h.Value,
				})
			}

			// Convert pieces
			if f3.Verification.Pieces != nil {
				file.Pieces = &Pieces{
					Type:   f3.Verification.Pieces.Type,
					Length: f3.Verification.Pieces.Length,
				}
				for _, ph := range f3.Verification.Pieces.Hashes {
					file.Pieces.Hashes = append(file.Pieces.Hashes, PieceHash{
						Piece: ph.Piece,
						Value: ph.Value,
					})
				}
			}
		}

		// Convert resources to URLs
		for _, res := range f3.Resources {
			// Convert preference (1-100, higher is better) to priority (1-999, lower is better)
			priority := 100 - res.Preference + 1
			if priority < 1 {
				priority = 1
			}
			file.URLs = append(file.URLs, URL{
				Priority: priority,
				Location: res.Location,
				URL:      res.URL,
			})
		}

		files[i] = file
	}
	return files
}

// GetFile returns the first file entry (most common case: single file)
func (m *Metalink) GetFile() *File {
	if len(m.Files) > 0 {
		return &m.Files[0]
	}
	return nil
}

// GetFileByName returns a file entry by name
func (m *Metalink) GetFileByName(name string) *File {
	for i := range m.Files {
		if m.Files[i].Name == name {
			return &m.Files[i]
		}
	}
	return nil
}

// SortedURLs returns URLs sorted by priority (lower priority = higher preference)
func (f *File) SortedURLs() []URL {
	if len(f.URLs) == 0 {
		return nil
	}

	// Copy and sort
	urls := make([]URL, len(f.URLs))
	copy(urls, f.URLs)

	// Simple bubble sort (small lists)
	for i := 0; i < len(urls)-1; i++ {
		for j := 0; j < len(urls)-i-1; j++ {
			if urls[j].Priority > urls[j+1].Priority {
				urls[j], urls[j+1] = urls[j+1], urls[j]
			}
		}
	}

	return urls
}

// GetChecksum returns the checksum of the specified type
// Returns empty string if not found
func (f *File) GetChecksum(hashType string) string {
	hashType = strings.ToLower(hashType)
	for _, h := range f.Hashes {
		if strings.ToLower(h.Type) == hashType {
			return h.Value
		}
	}
	return ""
}

// GetPreferredChecksum returns the best available checksum
// Preference order: sha-512, sha-384, sha-256, sha-1, md5
func (f *File) GetPreferredChecksum() (hashType, value string) {
	preferences := []string{"sha-512", "sha-384", "sha-256", "sha-1", "md5"}
	for _, pref := range preferences {
		if v := f.GetChecksum(pref); v != "" {
			return pref, v
		}
	}
	return "", ""
}

// IsMetalink checks if a filename appears to be a metalink file
func IsMetalink(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".metalink") || strings.HasSuffix(lower, ".meta4")
}
