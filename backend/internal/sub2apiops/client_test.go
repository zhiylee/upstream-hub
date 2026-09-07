package sub2apiops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNormalizeSiteURL(t *testing.T) {
	t.Parallel()

	got, err := NormalizeSiteURL("  https://example.com/sub2api/  ")
	if err != nil {
		t.Fatalf("NormalizeSiteURL returned error: %v", err)
	}
	if got != "https://example.com/sub2api" {
		t.Fatalf("NormalizeSiteURL = %q", got)
	}
	for _, raw := range []string{
		"",
		"ftp://example.com",
		"https://user:pass@example.com",
		"https://example.com?key=value",
		"https://example.com/#fragment",
	} {
		if _, err := NormalizeSiteURL(raw); err == nil {
			t.Errorf("NormalizeSiteURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestClientDoUsesAdminPathAndKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/ops/dashboard/snapshot-v2" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("time_range") != "1h" {
			t.Errorf("time_range = %q", r.URL.Query().Get("time_range"))
		}
		if r.Header.Get("X-API-Key") != "admin-secret" {
			t.Errorf("X-API-Key = %q", r.Header.Get("X-API-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"ok":true}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	payload, status, err := client.Do(
		context.Background(),
		http.MethodGet,
		"/ops/dashboard/snapshot-v2",
		url.Values{"time_range": {"1h"}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || !strings.Contains(string(payload), `"ok":true`) {
		t.Fatalf("status=%d payload=%s", status, payload)
	}
}

func TestClientDoesNotFollowRedirectWithAdminKey(t *testing.T) {
	t.Parallel()

	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "" {
			leaked.Store(true)
		}
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client, err := NewClient(source.URL, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.Do(context.Background(), http.MethodGet, "/groups/all", nil, nil)
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 RemoteError, got %v", err)
	}
	if leaked.Load() {
		t.Fatal("admin key leaked to redirect target")
	}
}

func TestClientTestValidatesComplianceAndProtectedRoute(t *testing.T) {
	t.Parallel()

	required := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/compliance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"required": required},
			})
		case "/api/v1/admin/groups/all":
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Test(context.Background()); err != nil {
		t.Fatalf("valid connection failed: %v", err)
	}
	required = true
	if err := client.Test(context.Background()); err == nil || !strings.Contains(err.Error(), "合规") {
		t.Fatalf("expected compliance error, got %v", err)
	}
}
