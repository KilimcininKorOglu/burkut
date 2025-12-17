// Package version provides build-time version information for Burkut.
package version

import (
	"fmt"
	"runtime"
)

// These variables are set at build time using -ldflags
var (
	// Version is the semantic version (e.g., "0.1.0")
	Version = "dev"

	// Commit is the git commit hash
	Commit = "unknown"

	// Date is the build date
	Date = "unknown"
)

// Info contains complete version information
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the complete version information
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf("Burkut %s (%s) built on %s with %s",
		i.Version, i.Commit, i.Date, i.GoVersion)
}

// Short returns a short version string
func Short() string {
	return fmt.Sprintf("burkut %s", Version)
}

// Full returns the full version string
func Full() string {
	return Get().String()
}
