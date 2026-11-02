package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// FileStore stores OAuth credentials separately from ordinary configuration.
// Reads reject nonregular files and check that the opened file still matches
// the path. Except for logout and Windows, group or other permissions are
// rejected. Windows relies on filesystem access controls instead of Unix mode
// checks. Malformed file diagnostics do not include secret input.
type FileStore struct{ path string }

// Source returns the credential source name.
func (s *FileStore) Source() string { return "file" }

// Path returns the credential file path.
func (s *FileStore) Path() string { return s.path }

// checkFile checks that the path is a regular file and, when requested on
// non-Windows systems, has no group or other permission bits.
func (s *FileStore) checkFile(checkPermissions bool) error {
	info, err := os.Lstat(s.path)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("authentication file must be a regular file: %w", ErrInvalidInput)
	}

	if checkPermissions && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return s.insecurePermissions()
	}

	return nil
}

// insecurePermissions returns an error with a command to restrict file access.
func (s *FileStore) insecurePermissions() error {
	return fmt.Errorf("authentication file permissions are insecure; run chmod 600 on %s: %w", s.path, ErrInvalidInput)
}

// LoadToken reads credentials after checking file permissions.
func (s *FileStore) LoadToken() (*Token, error) { return s.loadToken(true) }

// LoadTokenForLogout reads a regular credential file without checking permissions.
func (s *FileStore) LoadTokenForLogout() (*Token, error) { return s.loadToken(false) }

// loadToken validates the path and opened file before reading credentials.
// It omits parser input from errors to avoid exposing secrets.
func (s *FileStore) loadToken(checkPermissions bool) (*Token, error) {
	if err := s.checkFile(checkPermissions); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("not authenticated - run 'xf auth login': %w", ErrAuthRequired)
		}

		return nil, err
	}

	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read authentication file: %w", err)
	}

	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	pathInfo, err := os.Lstat(s.path)
	if err != nil {
		return nil, err
	}

	if !info.Mode().IsRegular() || !pathInfo.Mode().IsRegular() || !os.SameFile(info, pathInfo) {
		return nil, fmt.Errorf("authentication file changed while opening: %w", ErrInvalidInput)
	}

	if checkPermissions && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, s.insecurePermissions()
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read authentication file: %w", err)
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("invalid authentication file; run 'xf auth login' to replace it: %w", ErrInvalidInput)
	}

	if token.AccessToken == "" || token.BaseURL == "" {
		return nil, fmt.Errorf("authentication file is missing token data: %w", ErrInvalidInput)
	}

	return &token, nil
}

// SaveToken writes nonexternal credentials to a temporary file and renames it
// over the destination after syncing it. Existing files must pass validation.
func (s *FileStore) SaveToken(token *Token) error {
	if token == nil || token.AccessToken == "" || token.BaseURL == "" || token.External {
		return fmt.Errorf("invalid token for persistent storage: %w", ErrInvalidInput)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("failed to create authentication directory: %w", err)
	}

	if err := s.checkFile(true); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to encode authentication token: %w", err)
	}

	file, err := os.CreateTemp(filepath.Dir(s.path), ".auth-*")
	if err != nil {
		return fmt.Errorf("failed to create authentication file: %w", err)
	}

	defer func() { _ = os.Remove(file.Name()) }()
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write authentication file: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync authentication file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close authentication file: %w", err)
	}

	if err := os.Rename(file.Name(), s.path); err != nil {
		return fmt.Errorf("failed to replace authentication file: %w", err)
	}

	return nil
}

// DeleteToken removes a regular credential file, ignoring a missing file.
func (s *FileStore) DeleteToken() error {
	if err := s.checkFile(false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove authentication file: %w", err)
	}

	return nil
}

// PrepareLogin rejects known storage failures before browser authentication.
// The temporary probe does not replace existing credentials.
func (s *FileStore) PrepareLogin() error {
	if err := s.checkFile(true); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("failed to create authentication directory: %w", err)
	}

	probe, err := os.CreateTemp(filepath.Dir(s.path), ".auth-check-*")
	if err != nil {
		return fmt.Errorf("authentication directory is not writable: %w", err)
	}

	defer func() { _ = os.Remove(probe.Name()) }()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("failed to close authentication storage probe: %w", err)
	}

	return nil
}
