package initialize

import (
	"strconv"
	"strings"

	"github.com/xenforo-ltd/cli/internal/customerapi"
)

// displayVersion represents a version option for display.
type displayVersion struct {
	value int
	label string
}

// buildVersionOptions builds display options for a version selection.
func buildVersionOptions(versions []customerapi.Version, maxVersion int) []displayVersion {
	if len(versions) == 0 {
		return nil
	}

	if maxVersion <= 0 || maxVersion > len(versions) {
		maxVersion = len(versions)
	}

	latestID := versions[0].VersionID

	out := make([]displayVersion, 0, maxVersion)
	for _, v := range versions[:maxVersion] {
		label := v.VersionStr
		switch {
		case v.VersionID == latestID && !v.Stable:
			label += " (latest, pre-release)"
		case v.VersionID == latestID:
			label += " (latest)"
		case !v.Stable:
			label += " (pre-release)"
		}

		out = append(out, displayVersion{
			value: v.VersionID,
			label: label,
		})
	}

	return out
}

// resolveVersionInput resolves a version input string to a full version.
func resolveVersionInput(input string, versions []customerapi.Version) (customerapi.Version, bool) {
	input = strings.TrimSpace(strings.ToLower(input))

	input = strings.TrimPrefix(input, "v")
	if input == "" {
		return customerapi.Version{}, false
	}

	if id, err := strconv.Atoi(input); err == nil {
		for _, v := range versions {
			if v.VersionID == id {
				return v, true
			}
		}
	}

	for _, v := range versions {
		n := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(v.VersionStr)), "v")
		if n == input {
			return v, true
		}
	}

	return customerapi.Version{}, false
}
