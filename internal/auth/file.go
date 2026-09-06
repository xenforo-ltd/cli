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
type FileStore struct{ path string }

func (s *FileStore) Source() string { return "file" }

func (s *FileStore) Path() string { return s.path }

func (s *FileStore) checkFile(checkPermissions bool) error {
	info, err := os.Lstat(s.path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("authentication file must be a regular file: %w", ErrInvalidInput)
	}
	// Windows permissions are governed by the containing directory's ACL.
	if checkPermissions && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("authentication file permissions are insecure; run chmod 600 on %s: %w", s.path, ErrInvalidInput)
	}
	return nil
}

func (s *FileStore) LoadToken() (*Token, error) { return s.loadToken(true) }

func (s *FileStore) LoadTokenForLogout() (*Token, error) { return s.loadToken(false) }

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
	// Check the opened file as well as the path before reading any secrets.
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
		return nil, fmt.Errorf("authentication file permissions are insecure; run chmod 600 on %s: %w", s.path, ErrInvalidInput)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read authentication file: %w", err)
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		// Do not include parser input in diagnostics: this file contains secrets.
		return nil, fmt.Errorf("invalid authentication file; run 'xf auth login' to replace it: %w", ErrInvalidInput)
	}
	if token.AccessToken == "" || token.BaseURL == "" {
		return nil, fmt.Errorf("authentication file is missing token data: %w", ErrInvalidInput)
	}
	return &token, nil
}

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
	defer os.Remove(file.Name())
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
	defer os.Remove(probe.Name())
	if err := probe.Close(); err != nil {
		return fmt.Errorf("failed to close authentication storage probe: %w", err)
	}
	return nil
}
