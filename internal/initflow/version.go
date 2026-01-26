package initflow

import (
	"sort"
	"strconv"
	"strings"

	"xf/internal/api"
)

type DisplayVersion struct {
	Value int
	Label string
}

func SortVersionsDesc(versions []api.Version) {
	sort.Slice(versions, func(i, j int) bool {
		ti := versions[i].ReleaseDate.Time
		tj := versions[j].ReleaseDate.Time
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return versions[i].VersionID > versions[j].VersionID
	})
}

func BuildVersionOptions(versions []api.Version, max int) []DisplayVersion {
	if len(versions) == 0 {
		return nil
	}
	if max <= 0 || max > len(versions) {
		max = len(versions)
	}
	latestID := versions[0].VersionID
	out := make([]DisplayVersion, 0, max)
	for _, v := range versions[:max] {
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

func ResolveVersionInput(input string, versions []api.Version) (api.Version, bool) {
	input = strings.TrimSpace(strings.ToLower(input))
	input = strings.TrimPrefix(input, "v")
	if input == "" {
		return api.Version{}, false
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
	return api.Version{}, false
}
