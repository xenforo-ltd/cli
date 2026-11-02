package xf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// metadataFilename is the installation's CLI metadata file name.
const metadataFilename = ".xf.json"

// Metadata stores CLI-specific information about a XenForo installation.
// It records the license key, Docker instance name, installed products and
// version, and creation and update times for use by init and upgrade.
type Metadata struct {
	LicenseKey string `json:"license_key"`

	InstanceName string `json:"instance_name"`

	InstalledProducts []string `json:"installed_products"`

	InstalledVersion string `json:"installed_version"`

	InstalledVersionID int `json:"installed_version_id"`

	CreatedAt time.Time `json:"created_at"`

	UpdatedAt time.Time `json:"updated_at"`
}

// metadataPath returns the path to the metadata file.
func metadataPath(xfDir string) string {
	return filepath.Join(xfDir, metadataFilename)
}

// ReadMetadata reads the metadata file. Returns ErrMetadataNotFound if the file doesn't exist.
func ReadMetadata(xfDir string) (*Metadata, error) {
	metaPath := metadataPath(xfDir)

	data, err := os.ReadFile(metaPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrMetadataNotFound
		}

		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata file: %w", err)
	}

	return &meta, nil
}

// WriteMetadata updates the timestamps and writes metadata with mode 0600
// when creating the file. Existing file permissions are not changed.
func WriteMetadata(xfDir string, meta *Metadata) error {
	metaPath := metadataPath(xfDir)

	meta.UpdatedAt = time.Now()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = meta.UpdatedAt
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}

	if err := os.WriteFile(metaPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
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
