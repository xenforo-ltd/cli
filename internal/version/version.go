// Package version provides CLI version information.
package version

import "runtime"

var (
	// Version is the CLI version, optionally set at link time.
	Version = "v0.0.0"
	// Commit is the source revision, optionally set at link time.
	Commit = "unknown"
	// Date is the build date, optionally set at link time.
	Date = "unknown"
)

// info contains version and build information.
type info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the current version information.
func Get() info {
	return info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}
