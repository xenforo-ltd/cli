package xf

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

const metadataFilename = ".xf.json"

// ErrMetadataNotFound is returned when the metadata file cannot be found.
var ErrMetadataNotFound = errors.New("metadata not found")

// Metadata stores CLI-specific information about a XenForo installation.
// This file is created during `init` and used by `upgrade` to remember settings.
type Metadata struct {
	// LicenseKey is the license key used for this installation
	LicenseKey string `json:"license_key"`

	// InstanceName is the Docker instance name
	InstanceName string `json:"instance_name"`

	// InstalledProducts tracks which products were installed
	InstalledProducts []string `json:"installed_products"`

	// InstalledVersion is the version that was installed
	InstalledVersion string `json:"installed_version"`

	// InstalledVersionID is the version ID that was installed
	InstalledVersionID int `json:"installed_version_id"`

	// CreatedAt is when the installation was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the metadata was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// MetadataPath returns the path to the metadata file.
func MetadataPath(xfDir string) string {
	return filepath.Join(xfDir, metadataFilename)
}

// ReadMetadata reads the metadata file. Returns nil (not an error) if the file doesn't exist.
func ReadMetadata(xfDir string) (*Metadata, error) {
	metaPath := MetadataPath(xfDir)

	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrMetadataNotFound
		}

		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to read metadata file", err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, clierrors.Wrap(clierrors.CodeInvalidInput, "failed to parse metadata file", err)
	}

	return &meta, nil
}

// WriteMetadata writes metadata to the metadata file.
func WriteMetadata(xfDir string, meta *Metadata) error {
	metaPath := MetadataPath(xfDir)

	meta.UpdatedAt = time.Now()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = meta.UpdatedAt
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return clierrors.Wrap(clierrors.CodeInvalidInput, "failed to serialize metadata", err)
	}

	if err := os.WriteFile(metaPath, data, 0o600); err != nil {
		return clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to write metadata file", err)
	}

	return nil
}

// UpdateMetadataVersion updates the version information in the metadata.
func UpdateMetadataVersion(xfDir string, version *Version) error {
	meta, err := ReadMetadata(xfDir)
	if err != nil && !errors.Is(err, ErrMetadataNotFound) {
		return err
	}

	if meta == nil {
		meta = &Metadata{}
	}

	meta.InstalledVersion = version.String
	meta.InstalledVersionID = version.ID

	return WriteMetadata(xfDir, meta)
}
