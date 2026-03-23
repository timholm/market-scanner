package db

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("failed to open temp db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Verify the DB file was created.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db file not created: %v", err)
	}

	// Open again to verify idempotent migration.
	d2, err := Open(path)
	if err != nil {
		t.Fatalf("Open (second time): %v", err)
	}
	d2.Close()
}

func TestSaveAndGetScan(t *testing.T) {
	d := tempDB(t)

	report := map[string]interface{}{
		"github": []string{"repo1", "repo2"},
		"score":  0.75,
	}

	err := d.SaveScan("test-product", "solves testing", 0.75, "PROCEED", report)
	if err != nil {
		t.Fatalf("SaveScan: %v", err)
	}

	result, err := d.GetScan("test-product")
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Name != "test-product" {
		t.Errorf("expected test-product, got %q", result.Name)
	}
	if result.Problem != "solves testing" {
		t.Errorf("expected 'solves testing', got %q", result.Problem)
	}
	if result.NoveltyScore != 0.75 {
		t.Errorf("expected 0.75, got %f", result.NoveltyScore)
	}
	if result.Recommendation != "PROCEED" {
		t.Errorf("expected PROCEED, got %q", result.Recommendation)
	}
	if result.ReportJSON == "" || result.ReportJSON == "{}" {
		t.Error("expected non-empty report JSON")
	}
}

func TestGetScanNotFound(t *testing.T) {
	d := tempDB(t)

	result, err := d.GetScan("nonexistent")
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %+v", result)
	}
}

func TestGetScanReturnsLatest(t *testing.T) {
	d := tempDB(t)

	_ = d.SaveScan("product", "p1", 0.5, "OLD", nil)
	_ = d.SaveScan("product", "p2", 0.9, "NEW", nil)

	result, err := d.GetScan("product")
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if result.Recommendation != "NEW" {
		t.Errorf("expected latest scan (NEW), got %q", result.Recommendation)
	}
}

func TestListScans(t *testing.T) {
	d := tempDB(t)

	for i := 0; i < 5; i++ {
		_ = d.SaveScan("product", "problem", float64(i)*0.1, "REC", nil)
	}

	results, err := d.ListScans(3)
	if err != nil {
		t.Fatalf("ListScans: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestListScansDefaultLimit(t *testing.T) {
	d := tempDB(t)

	_ = d.SaveScan("product", "p", 0.5, "REC", nil)

	results, err := d.ListScans(0)
	if err != nil {
		t.Fatalf("ListScans: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestBuildQueueFlow(t *testing.T) {
	d := tempDB(t)

	// Insert pending items directly.
	_, err := d.conn.Exec(
		`INSERT INTO build_queue (name, problem, status) VALUES (?, ?, 'pending')`,
		"tool-a", "problem a",
	)
	if err != nil {
		t.Fatalf("insert queue item: %v", err)
	}
	_, err = d.conn.Exec(
		`INSERT INTO build_queue (name, problem, status) VALUES (?, ?, 'pending')`,
		"tool-b", "problem b",
	)
	if err != nil {
		t.Fatalf("insert queue item: %v", err)
	}

	// Fetch pending items.
	items, err := d.PendingQueue()
	if err != nil {
		t.Fatalf("PendingQueue: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 pending items, got %d", len(items))
	}
	if items[0].Name != "tool-a" {
		t.Errorf("expected tool-a first, got %q", items[0].Name)
	}

	// Mark one as scanned.
	if err := d.MarkScanned(items[0].ID, "scanned_proceed"); err != nil {
		t.Fatalf("MarkScanned: %v", err)
	}

	// Only one pending now.
	remaining, err := d.PendingQueue()
	if err != nil {
		t.Fatalf("PendingQueue after mark: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 pending item, got %d", len(remaining))
	}
	if remaining[0].Name != "tool-b" {
		t.Errorf("expected tool-b remaining, got %q", remaining[0].Name)
	}
}

func TestPendingQueueEmpty(t *testing.T) {
	d := tempDB(t)

	items, err := d.PendingQueue()
	if err != nil {
		t.Fatalf("PendingQueue: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}
