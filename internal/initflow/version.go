package initflow

import (
	"sort"
	"strconv"
	"strings"

	"github.com/xenforo-ltd/cli/internal/customerapi"
)

// DisplayVersion represents a version option for display.
type DisplayVersion struct {
	Value int
	Label string
}

// SortVersionsDesc sorts versions by release date in descending order.
func SortVersionsDesc(versions []customerapi.Version) {
	sort.Slice(versions, func(i, j int) bool {
		ti := versions[i].ReleaseDate.Time

		tj := versions[j].ReleaseDate.Time
		if !ti.Equal(tj) {
			return ti.After(tj)
		}

		return versions[i].VersionID > versions[j].VersionID
	})
}

// BuildVersionOptions builds display options for a version selection.
func BuildVersionOptions(versions []customerapi.Version, maxVersion int) []DisplayVersion {
	if len(versions) == 0 {
		return nil
	}

	if maxVersion <= 0 || maxVersion > len(versions) {
		maxVersion = len(versions)
	}

	latestID := versions[0].VersionID

	out := make([]DisplayVersion, 0, maxVersion)
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

		out = append(out, DisplayVersion{
			Value: v.VersionID,
			Label: label,
		})
	}

	return out
}

// ResolveVersionInput resolves a version input string to a full version.
func ResolveVersionInput(input string, versions []customerapi.Version) (customerapi.Version, bool) {
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
