package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timholm/market-scanner/internal/config"
	"github.com/timholm/market-scanner/internal/db"
	"github.com/timholm/market-scanner/internal/scanner"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		ListenAddr:       ":0",
		NoveltyThreshold: 0.6,
		DBPath:           dbPath,
	}

	sc := scanner.New("", 0.6)
	return New(cfg, database, sc)
}

func TestHealthEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
	if body["service"] != "market-scanner" {
		t.Errorf("expected service market-scanner, got %q", body["service"])
	}
}

func TestReportsEmptyDB(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Should return null or empty array for no results.
	body := strings.TrimSpace(w.Body.String())
	if body != "null" && body != "[]" {
		t.Errorf("expected null or [], got %q", body)
	}
}

func TestReportNotFound(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/reports/nonexistent", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestScanPostBadRequest(t *testing.T) {
	srv := setupTestServer(t)

	// Missing required "name" field.
	req := httptest.NewRequest(http.MethodPost, "/scan",
		strings.NewReader(`{"problem": "test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing name, got %d: %s", w.Code, w.Body.String())
	}
}

func TestScanPostInvalidJSON(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/scan",
		strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRoutes(t *testing.T) {
	srv := setupTestServer(t)

	// Verify all expected routes are registered by hitting them.
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/reports"},
	}

	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Errorf("route %s %s returned 404 — not registered", rt.method, rt.path)
		}
	}
}

func TestScanGetEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}

	srv := setupTestServer(t)

	// GET /scan/:name makes an actual scan (may hit external APIs).
	// Just verify the route exists and responds.
	req := httptest.NewRequest(http.MethodGet, "/scan/zzz-nonexistent-tool?problem=test", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// We expect either 200 (scan succeeded) or 500 (scan failed due to network).
	// Anything but 404 proves the route is registered.
	if w.Code == http.StatusNotFound {
		t.Error("GET /scan/:name should be registered")
	}
}
