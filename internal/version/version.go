// Package version provides CLI version information.
package version

import (
	"fmt"
	"runtime"
)

// Version is the CLI version string.
var (
	Version = "v0.0.0"
	Commit  = "unknown"
	Date    = "unknown"
)

// Info contains version and build information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the current version information.
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

func (i Info) String() string {
	return fmt.Sprintf("xf %s (%s) built on %s\n%s %s/%s",
		i.Version, i.Commit, i.Date, i.GoVersion, i.OS, i.Arch)
}

// Short returns the version string.
func (i Info) Short() string {
	return i.Version
}
