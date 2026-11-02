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

// stability represents the release stability of a XenForo version.
type stability string

const (
	// stabilityAlpha identifies an alpha release.
	stabilityAlpha stability = "alpha"
	// stabilityBeta identifies a beta release.
	stabilityBeta stability = "beta"
	// stabilityRC identifies a release candidate.
	stabilityRC stability = "rc"
	// stabilityStable identifies a stable release.
	stabilityStable stability = "stable"
	// stabilityPL identifies a patch-level release.
	stabilityPL stability = "pl"
	// stabilityUnknown identifies an unrecognized stability code.
	stabilityUnknown stability = "unknown"

	// stabilityAlphaNumber is the alpha stability digit.
	stabilityAlphaNumber = 1
	// stabilityBetaNumber is the beta stability digit.
	stabilityBetaNumber = 3
	// stabilityRCNumber is the release-candidate stability digit.
	stabilityRCNumber = 5
	// stabilityStableNumber is the stable stability digit.
	stabilityStableNumber = 7
	// stabilityPLNumber is the patch-level stability digit.
	stabilityPLNumber = 9
)

var (
	// alphaNumberPattern extracts an alpha release number.
	alphaNumberPattern = regexp.MustCompile(`alpha\s*(\d+)`)
	// alphaStripPattern removes an alpha release suffix.
	alphaStripPattern = regexp.MustCompile(`(?i)\s*alpha\s*\d*`)
	// betaNumberPattern extracts a beta release number.
	betaNumberPattern = regexp.MustCompile(`beta\s*(\d+)`)
	// betaStripPattern removes a beta release suffix.
	betaStripPattern = regexp.MustCompile(`(?i)\s*beta\s*\d*`)
	// rcNumberPattern extracts a release-candidate number.
	rcNumberPattern = regexp.MustCompile(`(?:rc|release candidate)\s*(\d+)`)
	// rcStripPattern removes a release-candidate suffix.
	rcStripPattern = regexp.MustCompile(`(?i)\s*(?:rc|release candidate)\s*\d*`)
	// plNumberPattern extracts a patch-level number.
	plNumberPattern = regexp.MustCompile(`(?:pl|patch level)\s*(\d+)`)
	// plStripPattern removes a patch-level suffix.
	plStripPattern = regexp.MustCompile(`(?i)\s*(?:pl|patch level)\s*\d*`)
	// versionStrPattern extracts the version string from XF.php.
	versionStrPattern = regexp.MustCompile(`public\s+static\s+\$version\s*=\s*['"]([^'"]+)['"]`)
	// versionIDPattern extracts the version ID from XF.php.
	versionIDPattern = regexp.MustCompile(`public\s+static\s+\$versionId\s*=\s*(\d+)`)
)

// Version represents a XenForo version with both string and ID representations.
// Its ID encodes major, two-digit minor and patch, stability, and release level.
type Version struct {
	String    string
	ID        int
	major     int
	minor     int
	patch     int
	stability stability
	stabNum   int
	plLevel   int
}

// parseVersionString parses a version string into a Version structure.
func parseVersionString(versionStr string) (*Version, error) {
	v := &Version{String: versionStr}

	versionStr = strings.TrimSpace(versionStr)

	v.stability = stabilityStable
	v.stabNum = stabilityStableNumber
	v.plLevel = 1

	lowerStr := strings.ToLower(versionStr)
	var numberPattern, stripPattern *regexp.Regexp
	switch {
	case strings.Contains(lowerStr, string(stabilityAlpha)):
		v.stability = stabilityAlpha
		v.stabNum = stabilityAlphaNumber
		numberPattern, stripPattern = alphaNumberPattern, alphaStripPattern
	case strings.Contains(lowerStr, string(stabilityBeta)):
		v.stability = stabilityBeta
		v.stabNum = stabilityBetaNumber
		numberPattern, stripPattern = betaNumberPattern, betaStripPattern
	case strings.Contains(lowerStr, string(stabilityRC)) || strings.Contains(lowerStr, "release candidate"):
		v.stability = stabilityRC
		v.stabNum = stabilityRCNumber
		numberPattern, stripPattern = rcNumberPattern, rcStripPattern
	case strings.Contains(lowerStr, string(stabilityPL)) || strings.Contains(lowerStr, "patch level"):
		v.stability = stabilityPL
		v.stabNum = stabilityPLNumber
		numberPattern, stripPattern = plNumberPattern, plStripPattern
	}

	if numberPattern != nil {
		if matches := numberPattern.FindStringSubmatch(lowerStr); len(matches) > 1 {
			v.plLevel, _ = strconv.Atoi(matches[1])
		}

		versionStr = stripPattern.ReplaceAllString(versionStr, "")
	}

	versionStr = strings.TrimSpace(versionStr)

	parts := strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid version format %s: %w", v.String, errInvalidInput)
	}

	var err error

	v.major, err = strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version %s: %w", parts[0], err)
	}

	v.minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version %s: %w", parts[1], err)
	}

	if len(parts) >= 3 {
		v.patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid patch version %s: %w", parts[2], err)
		}
	}

	v.ID = v.major*1000000 + v.minor*10000 + v.patch*100 + v.stabNum*10 + v.plLevel

	return v, nil
}

// ParseVersionID converts a version ID into a Version structure.
func ParseVersionID(versionID int) *Version {
	v := &Version{ID: versionID}

	v.plLevel = versionID % 10
	versionID /= 10

	v.stabNum = versionID % 10
	versionID /= 10

	v.patch = versionID % 100
	versionID /= 100

	v.minor = versionID % 100
	versionID /= 100

	v.major = versionID

	switch v.stabNum {
	case stabilityAlphaNumber:
		v.stability = stabilityAlpha
	case stabilityBetaNumber:
		v.stability = stabilityBeta
	case stabilityRCNumber:
		v.stability = stabilityRC
	case stabilityStableNumber:
		v.stability = stabilityStable
	case stabilityPLNumber:
		v.stability = stabilityPL
	default:
		v.stability = stabilityUnknown
	}

	v.String = fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
	if v.stability != stabilityStable {
		switch v.stability {
		case stabilityAlpha:
			v.String += fmt.Sprintf(" Alpha %d", v.plLevel)
		case stabilityBeta:
			v.String += fmt.Sprintf(" Beta %d", v.plLevel)
		case stabilityRC:
			v.String += fmt.Sprintf(" RC %d", v.plLevel)
		case stabilityPL:
			v.String += fmt.Sprintf(" PL %d", v.plLevel)
		case stabilityUnknown:
			v.String += fmt.Sprintf(" (unknown %d)", v.stabNum)
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
// It prefers the numeric ID for parsing but retains the source version string;
// if no positive ID is found, it parses the version string instead.
func DetectVersionFS(installation fs.FS) (*Version, error) {
	file, err := installation.Open("src/XF.php")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("not a XenForo installation: src/XF.php not found: %w", err)
		}

		return nil, fmt.Errorf("failed to read XF.php: %w", err)
	}

	defer file.Close()

	var (
		versionStr string
		versionID  int
	)

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
		return nil, fmt.Errorf("could not detect XenForo version from XF.php: %w", errInvalidInput)
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

	if versionID > 0 {
		v := ParseVersionID(versionID)
		if versionStr != "" {
			v.String = versionStr
		}

		return v, nil
	}

	return parseVersionString(versionStr)
}
