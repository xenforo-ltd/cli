package customerapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
)

type stubTokenStore struct {
	mu    sync.Mutex
	token *auth.Token
}

func (s *stubTokenStore) LoadToken() (*auth.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.token, nil
}

func (s *stubTokenStore) SaveToken(token *auth.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.token = token

	return nil
}

func TestDoRetriesWithBody(t *testing.T) {
	body := []byte(`{"hello":"world"}`)

	var (
		attempts int
		received [][]byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		data, _ := io.ReadAll(r.Body)
		received = append(received, data)

		if attempts == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &stubTokenStore{token: &auth.Token{AccessToken: "old", RefreshToken: "refresh"}}
	client := &Client{
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
		keychain: store,
		oauthCfg: &config.OAuthConfig{BaseURL: server.URL, ClientID: "test"},
	}

	client.refreshFn = func(_ context.Context, _ string) error {
		return store.SaveToken(&auth.Token{AccessToken: "new", RefreshToken: "refresh"})
	}

	resp, err := client.Do(context.Background(), http.MethodPost, "/retry", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}

	resp.Body.Close()

	if len(received) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(received))
	}

	if !bytes.Equal(received[0], body) || !bytes.Equal(received[1], body) {
		t.Fatalf("expected identical body on retry")
	}
}

func TestRefreshTokenSingleFlight(t *testing.T) {
	var (
		refreshCalls int
		refreshMu    sync.Mutex
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/customer-oauth2/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		refreshMu.Lock()
		refreshCalls++
		refreshMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"new","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()

	store := &stubTokenStore{token: &auth.Token{AccessToken: "old", RefreshToken: "refresh"}}
	client := &Client{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: time.Second},
		keychain:   store,
		oauthCfg:   &config.OAuthConfig{BaseURL: server.URL, ClientID: "test"},
	}

	const goroutines = 2

	start := make(chan struct{})

	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			<-start

			_ = client.refreshToken(context.Background(), "old")
		})
	}

	close(start)
	wg.Wait()

	refreshMu.Lock()
	defer refreshMu.Unlock()

	if refreshCalls != 1 {
		t.Fatalf("expected 1 refresh call, got %d", refreshCalls)
	}
}
