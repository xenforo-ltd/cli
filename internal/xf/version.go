package xf

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
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

	stabilityAlphaNumber  = 1
	stabilityBetaNumber   = 3
	stabilityRCNumber     = 5
	stabilityStableNumber = 7
	stabilityPLNumber     = 9
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
	v.StabNum = stabilityStableNumber
	v.PLLevel = 1

	lowerStr := strings.ToLower(versionStr)
	switch {
	case strings.Contains(lowerStr, string(StabilityAlpha)):
		v.Stability = StabilityAlpha
		v.StabNum = stabilityAlphaNumber
		// Extract alpha number if present
		if matches := regexp.MustCompile(`alpha\s*(\d+)`).FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.PLLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = regexp.MustCompile(`(?i)\s*alpha\s*\d*`).ReplaceAllString(versionStr, "")
	case strings.Contains(lowerStr, string(StabilityBeta)):
		v.Stability = StabilityBeta

		v.StabNum = stabilityBetaNumber
		if matches := regexp.MustCompile(`beta\s*(\d+)`).FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.PLLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = regexp.MustCompile(`(?i)\s*beta\s*\d*`).ReplaceAllString(versionStr, "")
	case strings.Contains(lowerStr, string(StabilityRC)) || strings.Contains(lowerStr, "release candidate"):
		v.Stability = StabilityRC

		v.StabNum = stabilityRCNumber
		if matches := regexp.MustCompile(`(?:rc|release candidate)\s*(\d+)`).FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.PLLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = regexp.MustCompile(`(?i)\s*(?:rc|release candidate)\s*\d*`).ReplaceAllString(versionStr, "")
	case strings.Contains(lowerStr, string(StabilityPL)) || strings.Contains(lowerStr, "patch level"):
		v.Stability = StabilityPL

		v.StabNum = stabilityPLNumber
		if matches := regexp.MustCompile(`(?:pl|patch level)\s*(\d+)`).FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.PLLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = regexp.MustCompile(`(?i)\s*(?:pl|patch level)\s*\d*`).ReplaceAllString(versionStr, "")
	}

	versionStr = strings.TrimSpace(versionStr)

	parts := strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid version format %s: %w", v.String, ErrInvalidInput)
	}

	var err error

	v.Major, err = strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version %s: %w", parts[0], err)
	}

	v.Minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version %s: %w", parts[1], err)
	}

	if len(parts) >= 3 {
		v.Patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid patch version %s: %w", parts[2], err)
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
	case stabilityAlphaNumber:
		v.Stability = StabilityAlpha
	case stabilityBetaNumber:
		v.Stability = StabilityBeta
	case stabilityRCNumber:
		v.Stability = StabilityRC
	case stabilityStableNumber:
		v.Stability = StabilityStable
	case stabilityPLNumber:
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

// DetectVersion reads the core add-on version, checking it against src/XF.php.
// If the core add-on manifest is absent, it uses src/XF.php alone.
func DetectVersion(xfDir string) (*Version, error) {
	return DetectVersionFS(os.DirFS(xfDir))
}

// DetectVersionFS is DetectVersion for a filesystem rooted at the installation.
func DetectVersionFS(installation fs.FS) (*Version, error) {
	file, err := installation.Open("src/XF.php")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not a XenForo installation: src/XF.php not found: %w", err)
		}

		return nil, fmt.Errorf("failed to read XF.php: %w", err)
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
			versionID, err = strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("parsing src/XF.php version ID: %w", err)
			}
		}

		if versionStr != "" && versionID > 0 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read XF.php: %w", err)
	}

	if versionStr == "" && versionID == 0 {
		return nil, fmt.Errorf("could not detect XenForo version from XF.php: %w", ErrInvalidInput)
	}

	data, err := fs.ReadFile(installation, "src/addons/XF/addon.json")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading core addon.json: %w", err)
	}
	if err == nil {
		var metadata struct {
			VersionID     int    `json:"version_id"`
			VersionString string `json:"version_string"`
		}
		if err := json.Unmarshal(data, &metadata); err != nil {
			return nil, fmt.Errorf("parsing core addon.json: %w", err)
		}
		if metadata.VersionID <= 0 || metadata.VersionString == "" {
			return nil, errors.New("core addon.json must contain a positive version_id and a nonempty version_string")
		}
		if metadata.VersionID != versionID || metadata.VersionString != versionStr {
			return nil, fmt.Errorf("core version mismatch: src/XF.php reports %s [%d], but src/addons/XF/addon.json reports %s [%d]",
				versionStr, versionID, metadata.VersionString, metadata.VersionID)
		}
		versionStr, versionID = metadata.VersionString, metadata.VersionID
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
