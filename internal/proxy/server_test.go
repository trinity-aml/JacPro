package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := &SettingsStore{s: Settings{
		BaseURL:        "http://127.0.0.1:9117",
		Version:        Version,
		MergeV1:        true,
		EnrichTitles:   true,
		RequestTimeout: 20,
		LogLevel:       "CRITICAL",
		Host:           "127.0.0.1",
		Port:           5002,
	}}
	logger := testLogger(t)
	return NewServer(store, logger)
}

func TestTorznabCapsRoute(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "http://proxy.test/api?t=caps", nil)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<caps>") || !strings.Contains(body, "proxy.test/api/v2.0/indexers/all/results/torznab/api") {
		t.Fatalf("unexpected caps body: %s", body)
	}
}

func TestJackettIndexersRoute(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "http://proxy.test/api/v2.0/indexers", nil)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"id": "all"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestSettingsAPIPostUpdatesStore(t *testing.T) {
	server := newTestServer(t)
	body := strings.NewReader(`{"base_url":"http://jacred:9117","request_timeout":30,"merge_v1":false,"enrich_titles":false,"host":"127.0.0.1","port":5003,"log_level":"ERROR"}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy.test/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	settings := server.store.Get()
	if settings.BaseURL != "http://jacred:9117" || settings.MergeV1 || settings.EnrichTitles || settings.Port != 5003 {
		t.Fatalf("settings not updated: %#v", settings)
	}
}
