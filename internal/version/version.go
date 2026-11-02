// Package version provides CLI version information.
package version

import "runtime"

// Build metadata, set at link time.
var (
	Version = "v0.0.0"
	Commit  = "unknown"
	Date    = "unknown"
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
