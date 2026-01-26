package api

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"time"

	"xf/internal/errors"
)

// UnixTime is a time.Time that unmarshals from Unix timestamps.
type UnixTime struct {
	time.Time
}

func (t *UnixTime) UnmarshalJSON(data []byte) error {
	var timestamp int64
	if err := json.Unmarshal(data, &timestamp); err != nil {
		// Try parsing as a regular time string
		var timeStr string
		if err := json.Unmarshal(data, &timeStr); err != nil {
			return err
		}
		parsed, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			return err
		}
		t.Time = parsed
		return nil
	}

	if timestamp == 0 {
		t.Time = time.Time{}
	} else {
		t.Time = time.Unix(timestamp, 0)
	}
	return nil
}

// LicenseExtra represents an extra (add-on product) associated with a license.
type LicenseExtra struct {
	ExtraID        string   `json:"extra_id"`
	Name           string   `json:"name"`
	StartDate      UnixTime `json:"start_date"`
	IsDownloadable bool     `json:"is_downloadable"`
}

// License represents a XenForo customer license.
type License struct {
	LicenseID      int            `json:"license_id"`
	LicenseKey     string         `json:"license_key"`
	ProductID      string         `json:"product_id"`
	ProductTitle   string         `json:"product_title"`
	IsValid        bool           `json:"is_valid"`
	IsActive       bool           `json:"is_active"`
	StartDate      UnixTime       `json:"start_date"`
	ExpirationDate UnixTime       `json:"expiration_date,omitempty"`
	SiteURL        string         `json:"site_url,omitempty"`
	SiteTitle      string         `json:"site_title,omitempty"`
	CanDownload    bool           `json:"can_download"`
	Extras         []LicenseExtra `json:"extras,omitempty"`
}

// LicensesResponse represents the API response for listing licenses.
type LicensesResponse struct {
	Licenses []License `json:"licenses"`
}

func (c *Client) GetLicenses(ctx context.Context) ([]License, error) {
	resp, err := c.Get(ctx, "/api/customer-oauth2/licenses")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := CheckResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(errors.CodeAPIResponseInvalid, "failed to read response", err)
	}

	var result LicensesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.Wrap(errors.CodeAPIResponseInvalid, "failed to parse response", err)
	}

	return result.Licenses, nil
}

// Downloadable represents a downloadable product associated with a license.
type Downloadable struct {
	DownloadID string `json:"download_id"`
	Title      string `json:"title"`
}

// Version represents a downloadable version.
type Version struct {
	VersionID   int      `json:"version_id"`
	VersionStr  string   `json:"version_string"`
	ReleaseDate UnixTime `json:"release_date"`
	Stable      bool     `json:"stable"`
}

// LicenseDownloadables represents the downloadables available for a license.
type LicenseDownloadables struct {
	LicenseKey    string         `json:"license_key"`
	Downloadables []Downloadable `json:"downloadables"`
}

// LicenseVersions represents the versions available for a license/download.
type LicenseVersions struct {
	LicenseKey string    `json:"license_key"`
	DownloadID string    `json:"download_id"`
	Versions   []Version `json:"versions"`
}

func (c *Client) GetLicenseDownloadables(ctx context.Context, licenseKey string) (*LicenseDownloadables, error) {
	params := url.Values{}
	params.Set("license_key", licenseKey)
	path := "/api/customer-oauth2/license-downloadables?" + params.Encode()

	resp, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := CheckResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(errors.CodeAPIResponseInvalid, "failed to read response", err)
	}

	var result LicenseDownloadables
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.Wrap(errors.CodeAPIResponseInvalid, "failed to parse response", err)
	}

	return &result, nil
}

func (c *Client) GetLicenseVersions(ctx context.Context, licenseKey string, downloadID string) (*LicenseVersions, error) {
	params := url.Values{}
	params.Set("license_key", licenseKey)
	params.Set("download_id", downloadID)
	path := "/api/customer-oauth2/license-versions?" + params.Encode()

	resp, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := CheckResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(errors.CodeAPIResponseInvalid, "failed to read response", err)
	}

	var result LicenseVersions
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.Wrap(errors.CodeAPIResponseInvalid, "failed to parse response", err)
	}

	return &result, nil
}

// DownloadInfo represents information about a downloadable file.
type DownloadInfo struct {
	LicenseKey    string `json:"license_key"`
	DownloadID    string `json:"download_id"`
	VersionID     int    `json:"version_id"`
	VersionString string `json:"version_string"`
	Filename      string `json:"filename"`
	DownloadURL   string `json:"download_url"`
}

type DownloadInfoResponse struct {
	Download DownloadInfo `json:"download"`
}

func (c *Client) GetDownloadInfo(ctx context.Context, licenseKey string, downloadID string, versionID int) (*DownloadInfo, error) {
	params := url.Values{}
	params.Set("license_key", licenseKey)
	params.Set("download_id", downloadID)
	params.Set("version_id", strconv.Itoa(versionID))
	path := "/api/customer-oauth2/license-download-info?" + params.Encode()

	resp, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := CheckResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(errors.CodeAPIResponseInvalid, "failed to read response", err)
	}

	var result DownloadInfoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.Wrap(errors.CodeAPIResponseInvalid, "failed to parse response", err)
	}

	return &result.Download, nil
}

func (c *Client) GetDownloadURL(licenseKey string, downloadID string, versionID int) string {
	params := url.Values{}
	params.Set("license_key", licenseKey)
	params.Set("download_id", downloadID)
	params.Set("version_id", strconv.Itoa(versionID))
	return c.baseURL + "/api/customer-oauth2/license-download?" + params.Encode()
}

func (c *Client) GetAccessToken() (string, error) {
	token, err := c.keychain.LoadToken()
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
