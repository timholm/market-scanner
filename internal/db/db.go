package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ScanResult is a persisted scan for a product name.
type ScanResult struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Problem        string    `json:"problem"`
	NoveltyScore   float64   `json:"novelty_score"`
	Recommendation string    `json:"recommendation"`
	ReportJSON     string    `json:"report_json"`
	ScannedAt      time.Time `json:"scanned_at"`
}

// DB wraps SQLite access for market-scanner.
type DB struct {
	conn *sql.DB
}

// Open creates or opens the SQLite database at path.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=10000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{conn: conn}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

func migrate(conn *sql.DB) error {
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS scan_results (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			name            TEXT NOT NULL,
			problem         TEXT NOT NULL DEFAULT '',
			novelty_score   REAL NOT NULL DEFAULT 0,
			recommendation  TEXT NOT NULL DEFAULT '',
			report_json     TEXT NOT NULL DEFAULT '{}',
			scanned_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_scan_name ON scan_results(name);

		CREATE TABLE IF NOT EXISTS build_queue (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL,
			problem     TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'pending',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			scanned_at  DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_queue_status ON build_queue(status);
	`)
	return err
}

// SaveScan persists a scan result.
func (d *DB) SaveScan(name, problem string, noveltyScore float64, recommendation string, report interface{}) error {
	rj, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	_, err = d.conn.Exec(
		`INSERT INTO scan_results (name, problem, novelty_score, recommendation, report_json) VALUES (?, ?, ?, ?, ?)`,
		name, problem, noveltyScore, recommendation, string(rj),
	)
	return err
}

// GetScan returns the latest scan for a name, or nil if not found.
func (d *DB) GetScan(name string) (*ScanResult, error) {
	row := d.conn.QueryRow(
		`SELECT id, name, problem, novelty_score, recommendation, report_json, scanned_at
		 FROM scan_results WHERE name = ? ORDER BY id DESC LIMIT 1`, name,
	)
	var s ScanResult
	err := row.Scan(&s.ID, &s.Name, &s.Problem, &s.NoveltyScore, &s.Recommendation, &s.ReportJSON, &s.ScannedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListScans returns the most recent scan results, up to limit.
func (d *DB) ListScans(limit int) ([]ScanResult, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.conn.Query(
		`SELECT id, name, problem, novelty_score, recommendation, report_json, scanned_at
		 FROM scan_results ORDER BY scanned_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ScanResult
	for rows.Next() {
		var s ScanResult
		if err := rows.Scan(&s.ID, &s.Name, &s.Problem, &s.NoveltyScore, &s.Recommendation, &s.ReportJSON, &s.ScannedAt); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// PendingQueue returns build_queue entries with status 'pending'.
func (d *DB) PendingQueue() ([]BuildQueueItem, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, problem, status, created_at FROM build_queue WHERE status = 'pending' ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []BuildQueueItem
	for rows.Next() {
		var it BuildQueueItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Problem, &it.Status, &it.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// MarkScanned updates a build_queue item's status after scanning.
func (d *DB) MarkScanned(id int64, status string) error {
	_, err := d.conn.Exec(
		`UPDATE build_queue SET status = ?, scanned_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id,
	)
	return err
}

// BuildQueueItem represents an item in the build queue.
type BuildQueueItem struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Problem   string    `json:"problem"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
