// Package cache manages downloaded files and their metadata.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/config"
)

// Manager manages cache storage on disk.
type Manager struct {
	basePath string
}

// EntryMetadata holds information about a cached file.
type EntryMetadata struct {
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
	LicenseKey   string
	Metadata     EntryMetadata
	FilePath     string
	MetadataPath string
}

// NewManager creates a new cache manager.
func NewManager() (*Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load cache configuration: %w", err)
	}

	return &Manager{basePath: cfg.CachePath}, nil
}

// BasePath returns the cache manager's base directory.
func (m *Manager) BasePath() string {
	return m.basePath
}

// EntryPath returns the directory path for a cache entry.
func (m *Manager) EntryPath(licenseKey string, downloadID, version string) (string, error) {
	p := filepath.Clean(filepath.Join(m.basePath, licenseKey, downloadID, version))
	if !strings.HasPrefix(p, m.basePath+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid cache path components: %w", ErrInvalidInput)
	}

	return p, nil
}

// MetadataFilename is the name of the metadata file for each cache entry.
const MetadataFilename = ".metadata.json"

// GetEntry retrieves a cache entry by license, download ID, and version.
func (m *Manager) GetEntry(licenseKey string, downloadID, version string) (*Entry, error) {
	entryPath, err := m.EntryPath(licenseKey, downloadID, version)
	if err != nil {
		return nil, err
	}

	metadataPath := filepath.Join(entryPath, MetadataFilename)

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCacheMiss
		}

		return nil, fmt.Errorf("failed to read cache metadata: %w", err)
	}

	var metadata EntryMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse cache metadata: %w", err)
	}

	filePath := filepath.Join(entryPath, metadata.Filename)

	return &Entry{
		LicenseKey:   licenseKey,
		Metadata:     metadata,
		FilePath:     filePath,
		MetadataPath: metadataPath,
	}, nil
}

// SaveMetadata saves cache entry metadata to disk.
func (m *Manager) SaveMetadata(licenseKey string, metadata *EntryMetadata) error {
	entryPath, err := m.EntryPath(licenseKey, metadata.DownloadID, metadata.Version)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(entryPath, 0o750); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	metadataPath := filepath.Join(entryPath, MetadataFilename)

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

	checksum, err := CalculateChecksum(entry.FilePath)
	if err != nil {
		return false, err
	}

	return checksum == entry.Metadata.Checksum, nil
}

// Delete removes a cache entry.
func (m *Manager) Delete(licenseKey string, downloadID, version string) error {
	entryPath, err := m.EntryPath(licenseKey, downloadID, version)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(entryPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete cache entry: %w", err)
	}

	m.cleanEmptyParents(entryPath)

	return nil
}

// PurgeAll removes all cached files.
func (m *Manager) PurgeAll() error {
	if err := os.RemoveAll(m.basePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to purge cache: %w", err)
	}

	return nil
}

// PurgeLicense removes all cached files for a specific license.
func (m *Manager) PurgeLicense(licenseKey string) error {
	licensePath := filepath.Clean(filepath.Join(m.basePath, licenseKey))
	if !strings.HasPrefix(licensePath, m.basePath+string(filepath.Separator)) {
		return fmt.Errorf("invalid license key: %w", ErrInvalidInput)
	}

	if err := os.RemoveAll(licensePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to purge license cache: %w", err)
	}

	return nil
}

// List returns all cached entries.
func (m *Manager) List() ([]*Entry, error) {
	var entries []*Entry

	if _, err := os.Stat(m.basePath); os.IsNotExist(err) {
		return entries, nil
	}

	err := filepath.WalkDir(m.basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Name() == MetadataFilename {
			entry, err := m.loadEntryFromMetadata(path)
			if err != nil {
				return err
			}

			if entry != nil {
				entries = append(entries, entry)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list cache: %w", err)
	}

	return entries, nil
}

// ListForLicense returns all cached entries for a specific license.
func (m *Manager) ListForLicense(licenseKey string) ([]*Entry, error) {
	var entries []*Entry

	licensePath := filepath.Clean(filepath.Join(m.basePath, licenseKey))
	if !strings.HasPrefix(licensePath, m.basePath+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid license key: %w", ErrInvalidInput)
	}

	if _, err := os.Stat(licensePath); os.IsNotExist(err) {
		return entries, nil
	}

	err := filepath.WalkDir(licensePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Name() == MetadataFilename {
			entry, err := m.loadEntryFromMetadata(path)
			if err != nil {
				return err
			}

			if entry != nil {
				entries = append(entries, entry)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list license cache: %w", err)
	}

	return entries, nil
}

// TotalSize returns the total size of all cached files in bytes.
func (m *Manager) TotalSize() (int64, error) {
	entries, err := m.List()
	if err != nil {
		return 0, err
	}

	var total int64
	for _, entry := range entries {
		total += entry.Metadata.Size
	}

	return total, nil
}

func (m *Manager) cleanEmptyParents(path string) {
	for {
		parent := filepath.Dir(path)
		if parent == m.basePath || parent == path {
			break
		}

		if err := os.Remove(parent); err != nil {
			break
		}

		path = parent
	}
}

func (m *Manager) loadEntryFromMetadata(metadataPath string) (*Entry, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache metadata %s: %w", metadataPath, err)
	}

	var metadata EntryMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse cache metadata %s: %w", metadataPath, err)
	}

	safeFilename := sanitizeFilename(metadata.Filename)
	if safeFilename == "" {
		return nil, fmt.Errorf("invalid cached filename %s: %w", metadata.Filename, ErrInvalidInput)
	}

	_, err = filepath.Rel(m.basePath, metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve relative cache path for %s: %w", metadataPath, err)
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
		LicenseKey:   licenseKey,
		Metadata:     metadata,
		FilePath:     filePath,
		MetadataPath: metadataPath,
	}, nil
}

// CalculateChecksum computes a file's SHA-256 checksum.
func CalculateChecksum(filePath string) (string, error) {
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
