// Package cache manages downloaded files and their metadata.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/xenforo-ltd/cli/internal/config"
)

// Manager manages cache storage on disk.
type Manager struct {
	basePath string
}

// entryMetadata holds information about a cached file.
type entryMetadata struct {
	DownloadID   string    `json:"download_id"`
	Version      string    `json:"version"`
	Filename     string    `json:"filename"`
	Checksum     string    `json:"checksum"`
	Size         int64     `json:"size"`
	DownloadedAt time.Time `json:"downloaded_at"`
	SourceURL    string    `json:"source_url,omitempty"`
}

// Entry represents a cached download file.
type Entry struct {
	LicenseKey string
	Metadata   entryMetadata
	FilePath   string
}

// NewManager creates a new cache manager.
func NewManager() (*Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load cache configuration: %w", err)
	}

	return &Manager{basePath: cfg.CachePath}, nil
}

// entryPath returns the directory path for a cache entry.
func (m *Manager) entryPath(licenseKey string, downloadID, version string) (string, error) {
	rel := filepath.Join(licenseKey, downloadID, version)
	if rel == "." || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("invalid cache path components: %w", errInvalidInput)
	}

	return filepath.Join(m.basePath, rel), nil
}

// metadataFilename is the name of the metadata file for each cache entry.
const metadataFilename = ".metadata.json"

// GetEntry retrieves a cache entry by license, download ID, and version.
func (m *Manager) GetEntry(licenseKey string, downloadID, version string) (*Entry, error) {
	entryPath, err := m.entryPath(licenseKey, downloadID, version)
	if err != nil {
		return nil, err
	}

	metadataPath := filepath.Join(entryPath, metadataFilename)

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errCacheMiss
		}

		return nil, fmt.Errorf("failed to read cache metadata: %w", err)
	}

	var metadata entryMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse cache metadata: %w", err)
	}

	filePath := filepath.Join(entryPath, metadata.Filename)

	return &Entry{
		LicenseKey: licenseKey,
		Metadata:   metadata,
		FilePath:   filePath,
	}, nil
}

type entryVerifier interface {
	GetEntry(licenseKey string, downloadID, version string) (*Entry, error)
	Verify(entry *Entry) (bool, error)
}

// FindValid returns the cached entry whose file exists and passes
// verification, or nil if there is none. Only a failure to look up the entry
// is returned as an error; a corrupt or missing file counts as no entry.
func FindValid(m entryVerifier, licenseKey string, downloadID, version string) (*Entry, error) {
	entry, err := m.GetEntry(licenseKey, downloadID, version)
	if err != nil {
		if errors.Is(err, errCacheMiss) {
			return nil, nil
		}

		return nil, err
	}

	if valid, err := m.Verify(entry); err != nil || !valid {
		return nil, nil
	}

	if _, err := os.Stat(entry.FilePath); err != nil {
		return nil, nil
	}

	return entry, nil
}

// saveMetadata saves cache entry metadata to disk.
func (m *Manager) saveMetadata(licenseKey string, metadata *entryMetadata) error {
	entryPath, err := m.entryPath(licenseKey, metadata.DownloadID, metadata.Version)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(entryPath, 0o750); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	metadataPath := filepath.Join(entryPath, metadataFilename)

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write cache metadata: %w", err)
	}

	return nil
}

// Verify checks if a cached file's checksum is valid.
func (m *Manager) Verify(entry *Entry) (bool, error) {
	if entry.Metadata.Checksum == "" {
		return true, nil
	}

	checksum, err := calculateChecksum(entry.FilePath)
	if err != nil {
		return false, err
	}

	return checksum == entry.Metadata.Checksum, nil
}

// PurgeAll removes all cached files.
func (m *Manager) PurgeAll() error {
	if err := os.RemoveAll(m.basePath); err != nil {
		return fmt.Errorf("failed to purge cache: %w", err)
	}

	return nil
}

// PurgeLicense removes all cached files for a specific license.
func (m *Manager) PurgeLicense(licenseKey string) error {
	licensePath, err := m.licensePath(licenseKey)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(licensePath); err != nil {
		return fmt.Errorf("failed to purge license cache: %w", err)
	}

	return nil
}

// List returns all cached entries.
func (m *Manager) List() ([]*Entry, error) {
	entries, err := m.walkEntries(m.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list cache: %w", err)
	}

	return entries, nil
}

// ListForLicense returns all cached entries for a specific license.
func (m *Manager) ListForLicense(licenseKey string) ([]*Entry, error) {
	licensePath, err := m.licensePath(licenseKey)
	if err != nil {
		return nil, err
	}

	entries, err := m.walkEntries(licensePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list license cache: %w", err)
	}

	return entries, nil
}

func (m *Manager) licensePath(licenseKey string) (string, error) {
	rel := filepath.Clean(licenseKey)
	if rel == "." || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("invalid license key: %w", errInvalidInput)
	}

	return filepath.Join(m.basePath, rel), nil
}

func (m *Manager) walkEntries(root string) ([]*Entry, error) {
	var entries []*Entry

	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return entries, nil
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Name() != metadataFilename {
			return nil
		}

		entry, err := m.loadEntryFromMetadata(path)
		if err != nil {
			return err
		}

		entries = append(entries, entry)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func (m *Manager) loadEntryFromMetadata(metadataPath string) (*Entry, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache metadata %s: %w", metadataPath, err)
	}

	var metadata entryMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse cache metadata %s: %w", metadataPath, err)
	}

	safeFilename := sanitizeFilename(metadata.Filename)
	if safeFilename == "" {
		return nil, fmt.Errorf("invalid cached filename %s: %w", metadata.Filename, errInvalidInput)
	}

	var licenseKey string

	dir := filepath.Dir(metadataPath)
	for {
		parent := filepath.Dir(dir)
		if parent == m.basePath {
			licenseKey = filepath.Base(dir)
			break
		}

		if parent == dir {
			break
		}

		dir = parent
	}

	entryDir := filepath.Dir(metadataPath)
	filePath := filepath.Join(entryDir, safeFilename)

	return &Entry{
		LicenseKey: licenseKey,
		Metadata:   metadata,
		FilePath:   filePath,
	}, nil
}

// calculateChecksum computes a file's SHA-256 checksum.
func calculateChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to read file for checksum: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
