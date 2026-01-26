package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"xf/internal/config"
	"xf/internal/errors"
)

// Manager handles download cache operations.
type Manager struct {
	basePath string
}

// EntryMetadata holds metadata about a cached download.
type EntryMetadata struct {
	// DownloadID is the identifier for the download (e.g., "xenforo", "xfmg").
	DownloadID string `json:"download_id"`

	// Version is the version string (e.g., "2.3.0").
	Version string `json:"version"`

	// Filename is the original filename of the download.
	Filename string `json:"filename"`

	// Checksum is the SHA-256 checksum of the file.
	Checksum string `json:"checksum"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// DownloadedAt is when the file was downloaded.
	DownloadedAt time.Time `json:"downloaded_at"`

	// SourceURL is the URL the file was downloaded from.
	SourceURL string `json:"source_url,omitempty"`
}

// Entry represents a cached file with its metadata.
type Entry struct {
	// LicenseKey is the license this download is associated with.
	LicenseKey string

	// Metadata contains the download metadata.
	Metadata EntryMetadata

	// FilePath is the full path to the cached file.
	FilePath string

	// MetadataPath is the full path to the metadata file.
	MetadataPath string
}

func NewManager() (*Manager, error) {
	basePath, err := GetCachePath()
	if err != nil {
		return nil, err
	}

	return &Manager{basePath: basePath}, nil
}

func GetCachePath() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}

	if cfg.CachePath != "" {
		return cfg.CachePath, nil
	}

	return config.DefaultCacheDir()
}

func (m *Manager) BasePath() string {
	return m.basePath
}

// sanitizePathComponent ensures a path component is safe for filesystem use.
// It only allows [A-Za-z0-9._-] and rejects path separators or "..".
func sanitizePathComponent(s string) (string, error) {
	if s == "" {
		return "", errors.New(errors.CodeValidationFailed, "path component cannot be empty")
	}

	if strings.ContainsAny(s, `/\\`) {
		return "", errors.Newf(errors.CodeValidationFailed, "invalid path component: %s", s)
	}

	if strings.Contains(s, "..") {
		return "", errors.Newf(errors.CodeValidationFailed, "invalid path component: %s", s)
	}

	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return "", errors.Newf(errors.CodeValidationFailed, "invalid path component: %s", s)
	}

	return s, nil
}

// Layout: cache/{license_key}/{download_id}/{version}/
func (m *Manager) EntryPath(licenseKey string, downloadID, version string) (string, error) {
	safeLicense, err := sanitizePathComponent(licenseKey)
	if err != nil {
		return "", err
	}

	safeDownload, err := sanitizePathComponent(downloadID)
	if err != nil {
		return "", err
	}

	safeVersion, err := sanitizePathComponent(version)
	if err != nil {
		return "", err
	}

	return filepath.Join(
		m.basePath,
		safeLicense,
		safeDownload,
		safeVersion,
	), nil
}

// MetadataFilename is the name of the metadata file in each cache entry directory.
const MetadataFilename = ".metadata.json"

// Returns nil, nil if the entry doesn't exist.
func (m *Manager) GetEntry(licenseKey string, downloadID, version string) (*Entry, error) {
	entryPath, err := m.EntryPath(licenseKey, downloadID, version)
	if err != nil {
		return nil, err
	}
	metadataPath := filepath.Join(entryPath, MetadataFilename)

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(errors.CodeFileReadFailed, "failed to read cache metadata", err)
	}

	var metadata EntryMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, errors.Wrap(errors.CodeConfigInvalid, "failed to parse cache metadata", err)
	}

	filePath := filepath.Join(entryPath, metadata.Filename)

	return &Entry{
		LicenseKey:   licenseKey,
		Metadata:     metadata,
		FilePath:     filePath,
		MetadataPath: metadataPath,
	}, nil
}

func (m *Manager) SaveMetadata(licenseKey string, metadata *EntryMetadata) error {
	entryPath, err := m.EntryPath(licenseKey, metadata.DownloadID, metadata.Version)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(entryPath, 0755); err != nil {
		return errors.Wrap(errors.CodeDirCreateFailed, "failed to create cache directory", err)
	}

	metadataPath := filepath.Join(entryPath, MetadataFilename)

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "failed to marshal metadata", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return errors.Wrap(errors.CodeFileWriteFailed, "failed to write cache metadata", err)
	}

	return nil
}

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

func (m *Manager) Delete(licenseKey string, downloadID, version string) error {
	entryPath, err := m.EntryPath(licenseKey, downloadID, version)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(entryPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(errors.CodeFileWriteFailed, "failed to delete cache entry", err)
	}

	m.cleanEmptyParents(entryPath)

	return nil
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

func (m *Manager) PurgeAll() error {
	if err := os.RemoveAll(m.basePath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(errors.CodeFileWriteFailed, "failed to purge cache", err)
	}
	return nil
}

func (m *Manager) PurgeLicense(licenseKey string) error {
	safeLicense, err := sanitizePathComponent(licenseKey)
	if err != nil {
		return err
	}
	licensePath := filepath.Join(m.basePath, safeLicense)

	if err := os.RemoveAll(licensePath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(errors.CodeFileWriteFailed, "failed to purge license cache", err)
	}
	return nil
}

func (m *Manager) List() ([]*Entry, error) {
	var entries []*Entry

	if _, err := os.Stat(m.basePath); os.IsNotExist(err) {
		return entries, nil
	}

	err := filepath.Walk(m.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.Name() == MetadataFilename {
			entry, err := m.loadEntryFromMetadata(path)
			if err == nil && entry != nil {
				entries = append(entries, entry)
			}
		}
		return nil
	})

	if err != nil {
		return nil, errors.Wrap(errors.CodeFileReadFailed, "failed to list cache", err)
	}

	return entries, nil
}

func (m *Manager) ListForLicense(licenseKey string) ([]*Entry, error) {
	var entries []*Entry

	safeLicense, err := sanitizePathComponent(licenseKey)
	if err != nil {
		return nil, err
	}
	licensePath := filepath.Join(m.basePath, safeLicense)

	if _, err := os.Stat(licensePath); os.IsNotExist(err) {
		return entries, nil
	}

	err = filepath.Walk(licensePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.Name() == MetadataFilename {
			entry, err := m.loadEntryFromMetadata(path)
			if err == nil && entry != nil {
				entries = append(entries, entry)
			}
		}
		return nil
	})

	if err != nil {
		return nil, errors.Wrap(errors.CodeFileReadFailed, "failed to list license cache", err)
	}

	return entries, nil
}

func (m *Manager) loadEntryFromMetadata(metadataPath string) (*Entry, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata EntryMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	_, err = filepath.Rel(m.basePath, metadataPath)
	if err != nil {
		return nil, err
	}

	var licenseKey string
	dir := filepath.Dir(metadataPath)
	for {
		parent := filepath.Dir(dir)
		if parent == m.basePath {
			// dir is the license_key directory
			licenseKey = filepath.Base(dir)
			break
		}
		if parent == dir {
			break
		}
		dir = parent
	}

	entryDir := filepath.Dir(metadataPath)
	filePath := filepath.Join(entryDir, metadata.Filename)

	return &Entry{
		LicenseKey:   licenseKey,
		Metadata:     metadata,
		FilePath:     filePath,
		MetadataPath: metadataPath,
	}, nil
}

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

func CalculateChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", errors.Wrap(errors.CodeFileReadFailed, "failed to open file for checksum", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errors.Wrap(errors.CodeFileReadFailed, "failed to read file for checksum", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
