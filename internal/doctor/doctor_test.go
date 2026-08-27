package doctor

import (
	"errors"
	"fmt"
	"testing"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func TestDoctorHasErrorsAndWarnings(t *testing.T) {
	d := &Doctor{results: []*CheckResult{{Status: StatusOK}}}
	if d.HasErrors() || d.HasWarnings() {
		t.Fatal("expected no errors or warnings")
	}

	d.results = append(d.results, &CheckResult{Status: StatusWarning})
	if d.HasErrors() {
		t.Fatal("expected no errors")
	}

	if !d.HasWarnings() {
		t.Fatal("expected warning")
	}

	d.results = append(d.results, &CheckResult{Status: StatusError})
	if !d.HasErrors() {
		t.Fatal("expected error")
	}
}

func TestFormatBytes(t *testing.T) {
	if got := ui.FormatBytes(512); got != "512 B" {
		t.Fatalf("FormatBytes(512) = %q", got)
	}

	if got := ui.FormatBytes(1024); got != "1.0 KB" {
		t.Fatalf("FormatBytes(1024) = %q", got)
	}

	if got := ui.FormatBytes(2 * 1024 * 1024); got != "2.0 MB" {
		t.Fatalf("FormatBytes(2MB) = %q", got)
	}
}

func TestEnvironmentAuthenticationSkipsKeychain(t *testing.T) {
	d := NewDoctor()
	d.checkAuthentication("environment", &auth.Token{External: true}, nil)
	if len(d.results) != 2 || d.results[0].Status != StatusOK {
		t.Fatal("environment credentials require keychain")
	}
	result := d.results[1]
	if result.Status != StatusOK || result.Message != "XF_TOKEN is present (validity and expiry not checked)" {
		t.Fatalf("unexpected auth diagnosis: %v", result)
	}
}

func TestCredentialFailuresAreNotReportedAsLoggedOut(t *testing.T) {
	for _, err := range []error{auth.ErrStoreUnavailable, fmt.Errorf("insecure credential file: %w", auth.ErrInvalidInput)} {
		d := NewDoctor()
		d.checkAuthentication("file", nil, err)
		if d.results[0].Name != "Credential Storage" || d.results[0].Status != StatusError || d.results[1].Status != StatusSkipped {
			t.Fatalf("misleading diagnosis: %+v / %+v", d.results[0], d.results[1])
		}
	}
	d := NewDoctor()
	d.checkAuthentication("file", nil, auth.ErrAuthRequired)
	if d.HasErrors() || d.results[1].Message != "Not authenticated" {
		t.Fatal("missing credentials reported as broken storage")
	}
}

// fakeStore is a read-only auth.Store used to exercise resolution without a
// real keychain, file, or environment.
type fakeStore struct {
	source string
}

func (f *fakeStore) Source() string                  { return f.source }
func (f *fakeStore) LoadToken() (*auth.Token, error) { return nil, nil }
func (f *fakeStore) SaveToken(*auth.Token) error     { return nil }
func (f *fakeStore) DeleteToken() error              { return nil }

// fakeWritableStore adds the persistent lifecycle that triggers preflight.
type fakeWritableStore struct {
	fakeStore
	prepareErr error
	prepared   bool
}

func (f *fakeWritableStore) PrepareLogin() error {
	f.prepared = true
	return f.prepareErr
}

func (f *fakeWritableStore) LoadTokenForLogout() (*auth.Token, error) { return nil, nil }

func TestResolveAuthenticationPreflightsWritableStores(t *testing.T) {
	preflightErr := errors.New("authentication directory is not writable")
	store := &fakeWritableStore{source: "file", prepareErr: preflightErr}
	loaded := false
	load := func(auth.Store) (*auth.Token, error) {
		loaded = true

		return &auth.Token{AccessToken: "token", BaseURL: "https://example.test"}, nil
	}

	source, token, err := resolveAuthentication(store, load)

	if !errors.Is(err, preflightErr) {
		t.Fatalf("err = %v, want preflight error", err)
	}

	if token != nil {
		t.Fatalf("token = %v, want nil after failed preflight", token)
	}

	if loaded {
		t.Fatal("credentials loaded despite failed preflight")
	}

	if !store.prepared {
		t.Fatal("writable store was not preflighted")
	}

	if source != "file" {
		t.Fatalf("source = %q, want %q", source, "file")
	}

	// Rendering must classify the preflight failure as broken storage.
	d := NewDoctor()
	d.checkAuthentication(source, token, err)
	if d.results[0].Status != StatusError || d.results[1].Status != StatusSkipped {
		t.Fatalf("preflight failure misdiagnosed: %+v / %+v", d.results[0], d.results[1])
	}
}

func TestResolveAuthenticationSkipsPreflightForReadOnlyStores(t *testing.T) {
	store := &fakeStore{source: "environment"}
	want := &auth.Token{AccessToken: "token", External: true}
	loaded := false
	load := func(auth.Store) (*auth.Token, error) {
		loaded = true

		return want, nil
	}

	source, token, err := resolveAuthentication(store, load)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !loaded {
		t.Fatal("read-only store bypassed credential loading")
	}

	if token != want {
		t.Fatalf("token = %v, want %v", token, want)
	}

	if source != "environment" {
		t.Fatalf("source = %q, want %q", source, "environment")
	}
}
