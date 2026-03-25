package customerapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
)

func TestGetLicenseDownloadablesEncodesQuery(t *testing.T) {
	store := &stubTokenStore{token: &auth.Token{AccessToken: "token", RefreshToken: "refresh"}}

	var requestURI string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURI = r.URL.RequestURI()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"license_key":"key","downloadables":[]}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{},
		keychain:   store,
		oauthCfg:   &config.OAuthConfig{BaseURL: server.URL, ClientID: "test"},
	}

	_, err := client.GetLicenseDownloadables(t.Context(), "ABC 123&x")
	if err != nil {
		t.Fatalf("GetLicenseDownloadables error: %v", err)
	}

	if !strings.Contains(requestURI, "license_key=ABC+123%26x") {
		t.Fatalf("expected encoded license_key, got %q", requestURI)
	}
}
