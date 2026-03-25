package xf

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

// Stability represents the release stability of a XenForo version.
type Stability string

// Stability constants represent different release stages.
const (
	StabilityAlpha   Stability = "alpha"
	StabilityBeta    Stability = "beta"
	StabilityRC      Stability = "rc"
	StabilityStable  Stability = "stable"
	StabilityPL      Stability = "pl"
	StabilityUnknown Stability = "unknown"
)

// Version represents a XenForo version with both string and ID representations.
type Version struct {
	String    string    // e.g., "2.3.8"
	ID        int       // e.g., 2030871
	Major     int       // e.g., 2
	Minor     int       // e.g., 3
	Patch     int       // e.g., 8
	Stability Stability // alpha, beta, rc, stable, pl
	StabNum   int       // stability number (1=alpha, 3=beta, 5=rc, 7=stable, 9=pl)
	PLLevel   int       // patch level (0-9)
}

// ParseVersionString parses a version string into a Version structure.
func ParseVersionString(versionStr string) (*Version, error) {
	v := &Version{String: versionStr}

	versionStr = strings.TrimSpace(versionStr)

	v.Stability = StabilityStable
	v.StabNum = 7
	v.PLLevel = 1

	lowerStr := strings.ToLower(versionStr)
	switch {
	case strings.Contains(lowerStr, string(StabilityAlpha)):
		v.Stability = StabilityAlpha
		v.StabNum = 1
		// Extract alpha number if present
		if matches := regexp.MustCompile(`alpha\s*(\d+)`).FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.PLLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = regexp.MustCompile(`(?i)\s*alpha\s*\d*`).ReplaceAllString(versionStr, "")
	case strings.Contains(lowerStr, string(StabilityBeta)):
		v.Stability = StabilityBeta

		v.StabNum = 3
		if matches := regexp.MustCompile(`beta\s*(\d+)`).FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.PLLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = regexp.MustCompile(`(?i)\s*beta\s*\d*`).ReplaceAllString(versionStr, "")
	case strings.Contains(lowerStr, string(StabilityRC)) || strings.Contains(lowerStr, "release candidate"):
		v.Stability = StabilityRC

		v.StabNum = 5
		if matches := regexp.MustCompile(`(?:rc|release candidate)\s*(\d+)`).FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.PLLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = regexp.MustCompile(`(?i)\s*(?:rc|release candidate)\s*\d*`).ReplaceAllString(versionStr, "")
	case strings.Contains(lowerStr, string(StabilityPL)) || strings.Contains(lowerStr, "patch level"):
		v.Stability = StabilityPL

		v.StabNum = 9
		if matches := regexp.MustCompile(`(?:pl|patch level)\s*(\d+)`).FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.PLLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = regexp.MustCompile(`(?i)\s*(?:pl|patch level)\s*\d*`).ReplaceAllString(versionStr, "")
	}

	versionStr = strings.TrimSpace(versionStr)

	parts := strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return nil, clierrors.Newf(clierrors.CodeInvalidInput, "invalid version format: %s", v.String)
	}

	var err error

	v.Major, err = strconv.Atoi(parts[0])
	if err != nil {
		return nil, clierrors.Newf(clierrors.CodeInvalidInput, "invalid major version: %s", parts[0])
	}

	v.Minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return nil, clierrors.Newf(clierrors.CodeInvalidInput, "invalid minor version: %s", parts[1])
	}

	if len(parts) >= 3 {
		v.Patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return nil, clierrors.Newf(clierrors.CodeInvalidInput, "invalid patch version: %s", parts[2])
		}
	}

	// Calculate version ID: abbccde
	// a = major, bb = minor, cc = patch, d = stability, e = pl level
	v.ID = v.Major*1000000 + v.Minor*10000 + v.Patch*100 + v.StabNum*10 + v.PLLevel

	return v, nil
}

// ParseVersionID converts a version ID into a Version structure.
func ParseVersionID(versionID int) *Version {
	v := &Version{ID: versionID}

	v.PLLevel = versionID % 10
	versionID /= 10

	v.StabNum = versionID % 10
	versionID /= 10

	v.Patch = versionID % 100
	versionID /= 100

	v.Minor = versionID % 100
	versionID /= 100

	v.Major = versionID

	switch v.StabNum {
	case 1:
		v.Stability = StabilityAlpha
	case 3:
		v.Stability = StabilityBeta
	case 5:
		v.Stability = StabilityRC
	case 7:
		v.Stability = StabilityStable
	case 9:
		v.Stability = StabilityPL
	default:
		v.Stability = StabilityUnknown
	}

	v.String = fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Stability != StabilityStable {
		switch v.Stability {
		case StabilityAlpha:
			v.String += fmt.Sprintf(" Alpha %d", v.PLLevel)
		case StabilityBeta:
			v.String += fmt.Sprintf(" Beta %d", v.PLLevel)
		case StabilityRC:
			v.String += fmt.Sprintf(" RC %d", v.PLLevel)
		case StabilityPL:
			v.String += fmt.Sprintf(" PL %d", v.PLLevel)
		case StabilityUnknown:
			v.String += fmt.Sprintf(" (unknown %d)", v.StabNum)
		case StabilityStable:
			break
		}
	}

	return v
}

// Compare returns -1 if v < other, 0 if v == other, 1 if v > other.
func (v *Version) Compare(other *Version) int {
	if v.ID < other.ID {
		return -1
	}

	if v.ID > other.ID {
		return 1
	}

	return 0
}

// IsNewerThan checks if this version is newer than another.
func (v *Version) IsNewerThan(other *Version) bool {
	return v.Compare(other) > 0
}

// IsOlderThan checks if this version is older than another.
func (v *Version) IsOlderThan(other *Version) bool {
	return v.Compare(other) < 0
}

// DetectVersion parses src/XF.php for the version string and ID.
func DetectVersion(xfDir string) (*Version, error) {
	xfPath := filepath.Join(xfDir, "src", "XF.php")

	file, err := os.Open(xfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, clierrors.New(clierrors.CodeFileNotFound, "not a XenForo installation: src/XF.php not found")
		}

		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to read XF.php", err)
	}
	defer file.Close()

	var (
		versionStr string
		versionID  int
	)

	versionStrPattern := regexp.MustCompile(`public\s+static\s+\$version\s*=\s*['"]([^'"]+)['"]`)
	versionIDPattern := regexp.MustCompile(`public\s+static\s+\$versionId\s*=\s*(\d+)`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if matches := versionStrPattern.FindStringSubmatch(line); len(matches) > 1 {
			versionStr = matches[1]
		}

		if matches := versionIDPattern.FindStringSubmatch(line); len(matches) > 1 {
			versionID, _ = strconv.Atoi(matches[1])
		}

		if versionStr != "" && versionID > 0 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to read XF.php", err)
	}

	if versionStr == "" && versionID == 0 {
		return nil, clierrors.New(clierrors.CodeInvalidInput, "could not detect XenForo version from XF.php")
	}

	// Prefer using the version ID as it's more precise
	if versionID > 0 {
		v := ParseVersionID(versionID)
		if versionStr != "" {
			v.String = versionStr // Use the actual string from the file
		}

		return v, nil
	}

	// Fall back to parsing the version string
	return ParseVersionString(versionStr)
}
