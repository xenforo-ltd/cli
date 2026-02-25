package downloads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/api"
	"github.com/xenforo-ltd/cli/internal/cache"
)

var (
	errTestNoToken     = errors.New("no token")
	errTestNetworkDown = errors.New("network down")
)

func v(id int, str string, ts string) api.Version {
	tm, _ := time.Parse(time.RFC3339, ts)

	return api.Version{
		VersionID:  id,
		VersionStr: str,
		ReleaseDate: api.UnixTime{
			Time: tm,
		},
	}
}

func TestResolveAddonSelection_LatestCoreUsesLatestAddon(t *testing.T) {
	addon := []api.Version{
		v(3, "2.3.9", "2025-10-01T00:00:00Z"),
		v(2, "2.3.8", "2025-09-01T00:00:00Z"),
	}
	core := &api.Version{VersionID: 100, VersionStr: "2.3.9"}
	latest := &api.Version{VersionID: 100, VersionStr: "2.3.9"}

	got := resolveAddonSelection(addon, core, latest, "2.3.9")
	if got.VersionID != 3 || got.Reason != "latest (core is latest)" {
		t.Fatalf("unexpected selection: %+v", got)
	}
}

func TestResolveAddonSelection_ExactMatch(t *testing.T) {
	addon := []api.Version{
		v(3, "2.3.10", "2025-11-01T00:00:00Z"),
		v(2, "2.3.8", "2025-09-01T00:00:00Z"),
		v(1, "2.3.5", "2025-06-01T00:00:00Z"),
	}
	core := &api.Version{VersionID: 99, VersionStr: "2.3.8"}
	latest := &api.Version{VersionID: 100, VersionStr: "2.3.9"}

	got := resolveAddonSelection(addon, core, latest, "2.3.8")
	if got.VersionID != 2 || got.Reason != "exact match" {
		t.Fatalf("unexpected selection: %+v", got)
	}
}

func TestResolveAddonSelection_DateFallback(t *testing.T) {
	addon := []api.Version{
		v(4, "2.3.10", "2025-11-01T00:00:00Z"),
		v(3, "2.3.6", "2025-07-01T00:00:00Z"),
		v(2, "2.3.5", "2025-06-01T00:00:00Z"),
	}
	core := &api.Version{
		VersionID:  98,
		VersionStr: "2.3.9",
		ReleaseDate: api.UnixTime{
			Time: mustTime("2025-07-15T00:00:00Z"),
		},
	}
	latest := &api.Version{VersionID: 100, VersionStr: "2.3.11"}

	got := resolveAddonSelection(addon, core, latest, "2.3.9")
	if got.VersionID != 3 || got.Reason != "date-match fallback" {
		t.Fatalf("unexpected selection: %+v", got)
	}
}

func TestResolveAddonSelection_LatestFallback(t *testing.T) {
	addon := []api.Version{
		v(5, "1.0.5", "2025-11-01T00:00:00Z"),
		v(4, "1.0.4", "2025-10-01T00:00:00Z"),
	}
	core := &api.Version{
		VersionID:  98,
		VersionStr: "2.1.0",
		ReleaseDate: api.UnixTime{
			Time: mustTime("2025-01-01T00:00:00Z"),
		},
	}
	latest := &api.Version{VersionID: 100, VersionStr: "2.3.11"}

	got := resolveAddonSelection(addon, core, latest, "2.1.0")
	if got.VersionID != 5 || got.Reason != "latest fallback (no <= release date)" {
		t.Fatalf("unexpected selection: %+v", got)
	}
}

func TestNormalizeVersion(t *testing.T) {
	if got := normalizeVersion(" v2.3.9 "); got != "2.3.9" {
		t.Fatalf("got %q", got)
	}

	if got := normalizeVersion("2030970"); got != "2030970" {
		t.Fatalf("got %q", got)
	}
}

func mustTime(s string) time.Time {
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}

	return tm
}

type fakeClient struct {
	versionsByProduct map[string][]api.Version
	downloadInfo      map[string]*api.DownloadInfo
	accessToken       string
	errVersions       map[string]error
	errDownloadInfo   map[string]error
	errAccessToken    error
}

func (f *fakeClient) GetLicenseVersions(_ context.Context, _ string, downloadID string) (*api.LicenseVersions, error) {
	if err := f.errVersions[downloadID]; err != nil {
		return nil, err
	}

	return &api.LicenseVersions{DownloadID: downloadID, Versions: append([]api.Version(nil), f.versionsByProduct[downloadID]...)}, nil
}

func (f *fakeClient) GetDownloadInfo(_ context.Context, _ string, downloadID string, versionID int) (*api.DownloadInfo, error) {
	key := fmt.Sprintf("%s:%d", downloadID, versionID)
	if err := f.errDownloadInfo[key]; err != nil {
		return nil, err
	}

	if info := f.downloadInfo[key]; info != nil {
		return info, nil
	}

	return &api.DownloadInfo{DownloadID: downloadID, VersionID: versionID, VersionString: "2.3.8", Filename: "x.zip"}, nil
}

func (f *fakeClient) GetAccessToken() (string, error) {
	if f.errAccessToken != nil {
		return "", f.errAccessToken
	}

	return f.accessToken, nil
}

func (f *fakeClient) GetDownloadURL(licenseKey string, downloadID string, versionID int) string {
	return fmt.Sprintf("https://example.com/%s/%s/%d", licenseKey, downloadID, versionID)
}

type fakeCache struct {
	entry            *cache.Entry
	verifyOK         bool
	verifyErr        error
	downloadResult   *cache.DownloadResult
	downloadErr      error
	downloadCalled   bool
	lastDownloadOpts cache.DownloadOptions
	lastAuthToken    string
}

func (f *fakeCache) GetEntry(_ string, _ string, _ string) (*cache.Entry, error) {
	return f.entry, nil
}

func (f *fakeCache) Verify(_ *cache.Entry) (bool, error) {
	return f.verifyOK, f.verifyErr
}

func (f *fakeCache) DownloadWithAuth(_ context.Context, opts cache.DownloadOptions, authToken string, _ cache.ProgressCallback) (*cache.DownloadResult, error) {
	f.downloadCalled = true
	f.lastDownloadOpts = opts

	f.lastAuthToken = authToken
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}

	return f.downloadResult, nil
}

func TestResolveSelections_RequiresCoreVersionID(t *testing.T) {
	client := &fakeClient{
		versionsByProduct: map[string][]api.Version{
			"xenforo": {v(10, "2.3.10", "2025-10-01T00:00:00Z")},
		},
	}

	_, err := ResolveSelections(context.Background(), client, "LIC", []string{"xenforo"}, 0, "", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveSelections_UsesOverrideAndSkipCallback(t *testing.T) {
	client := &fakeClient{
		versionsByProduct: map[string][]api.Version{
			"xenforo": {v(11, "2.3.11", "2025-11-01T00:00:00Z"), v(10, "2.3.10", "2025-10-01T00:00:00Z")},
			"xfmg":    {v(21, "2.3.11", "2025-11-02T00:00:00Z")},
			"xfes":    {},
		},
		downloadInfo: map[string]*api.DownloadInfo{
			"xfmg:99": {DownloadID: "xfmg", VersionID: 99, VersionString: "custom"},
		},
	}

	var skipped []string

	got, err := ResolveSelections(
		context.Background(),
		client,
		"LIC",
		[]string{"xenforo", "xfmg", "xfes"},
		10,
		"2.3.10",
		map[string]int{"xfmg": 99},
		func(product string) { skipped = append(skipped, product) },
	)
	if err != nil {
		t.Fatalf("ResolveSelections failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(selections) = %d, want 2", len(got))
	}

	if got[1].Product != "xfmg" || got[1].Reason != "manual override" || got[1].VersionID != 99 {
		t.Fatalf("unexpected override selection: %+v", got[1])
	}

	if len(skipped) != 1 || skipped[0] != "xfes" {
		t.Fatalf("skipped = %#v, want [xfes]", skipped)
	}
}

func TestDownloadSelection_Branches(t *testing.T) {
	ctx := context.Background()
	client := &fakeClient{
		accessToken: "token",
		downloadInfo: map[string]*api.DownloadInfo{
			"xenforo:10": {DownloadID: "xenforo", VersionID: 10, VersionString: "2.3.10", Filename: "xf.zip"},
		},
	}

	t.Run("returns cached entry", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "cached.zip")
		if err := os.WriteFile(p, []byte("cached"), 0o644); err != nil {
			t.Fatalf("write cached file: %v", err)
		}

		cached := &cache.Entry{FilePath: p}
		cacheMock := &fakeCache{entry: cached, verifyOK: true}

		entry, version, err := downloadSelection(ctx, client, cacheMock, "LIC", Selection{Product: "xenforo", VersionID: 10}, false, nil)
		if err != nil {
			t.Fatalf("downloadSelection failed: %v", err)
		}

		if entry != cached || version != "2.3.10" {
			t.Fatalf("unexpected cached result: entry=%v version=%q", entry, version)
		}

		if cacheMock.downloadCalled {
			t.Fatal("expected no download call when cache is valid")
		}
	})

	t.Run("downloads when cache missing", func(t *testing.T) {
		entryOut := &cache.Entry{FilePath: "/tmp/fake"}
		cacheMock := &fakeCache{
			verifyOK:       false,
			downloadResult: &cache.DownloadResult{Entry: entryOut},
		}

		entry, version, err := downloadSelection(ctx, client, cacheMock, "LIC", Selection{Product: "xenforo", VersionID: 10}, false, nil)
		if err != nil {
			t.Fatalf("downloadSelection failed: %v", err)
		}

		if !cacheMock.downloadCalled || cacheMock.lastAuthToken != "token" {
			t.Fatalf("expected download with auth token, called=%v token=%q", cacheMock.downloadCalled, cacheMock.lastAuthToken)
		}

		if entry != entryOut || version != "2.3.10" {
			t.Fatalf("unexpected download result: entry=%v version=%q", entry, version)
		}
	})

	t.Run("access token error", func(t *testing.T) {
		errClient := *client
		errClient.errAccessToken = errTestNoToken
		cacheMock := &fakeCache{}

		_, _, err := downloadSelection(ctx, &errClient, cacheMock, "LIC", Selection{Product: "xenforo", VersionID: 10}, true, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("download failure wraps", func(t *testing.T) {
		cacheMock := &fakeCache{
			downloadErr: errTestNetworkDown,
		}

		_, _, err := downloadSelection(ctx, client, cacheMock, "LIC", Selection{Product: "xenforo", VersionID: 10}, true, nil)
		if err == nil {
			t.Fatal("expected download error")
		}
	})
}
