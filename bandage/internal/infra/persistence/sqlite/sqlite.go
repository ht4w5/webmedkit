// Package sqlite is the sqlite report repository adapter (modernc.org/sqlite):
// one row per (rule, target) with a JSON body; reports survive restarts (ADR 0002).
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/domain/report"

	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

// MemoryPath names a throwaway in-memory database for Open: the store
// behind the daemon's memory mode (db.sqlite.memory), no files behind it.
const MemoryPath = ":memory:"

// ReportRepository is a report.Repository backed by a sqlite database.
type ReportRepository struct {
	// write is the sole writer (one connection): writes serialize
	// through it and land deterministically (ADR 0007).
	write *sql.DB
	// read serves queries on its own bounded pool (ADR 0009); for an
	// in-memory database it is write (each connection would be its own database).
	read *sql.DB
	// upsert is prepared once at Open and reused by every Upsert.
	upsert *sql.Stmt
}

// upsertSQL inserts the row or refreshes the mutable columns on conflict;
// created_at is excluded from the UPDATE set, so it survives without a pre-read.
const upsertSQL = `
INSERT INTO reports (rule, target, created_at, updated_at, expires_at, dynamic, body)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (rule, target) DO UPDATE SET
	updated_at = excluded.updated_at,
	expires_at = excluded.expires_at,
	dynamic    = excluded.dynamic,
	body       = excluded.body`

// Open opens (creating if needed) the report database at path and ensures the
// schema; file databases run WAL/synchronous=NORMAL (ADR 0009), MemoryPath does not split.
func Open(path string) (*ReportRepository, error) {
	write, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	write.SetMaxOpenConns(1)
	if !isMemory(path) {
		if _, err := write.Exec(`PRAGMA journal_mode=WAL`); err != nil {
			write.Close()
			return nil, fmt.Errorf("sqlite: journal mode: %w", err)
		}
	}
	// synchronous is per connection (only the write one commits);
	// NORMAL in WAL mode survives process crashes (ADR 0009).
	if _, err := write.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		write.Close()
		return nil, fmt.Errorf("sqlite: synchronous: %w", err)
	}
	if _, err := write.Exec(schema); err != nil {
		write.Close()
		return nil, fmt.Errorf("sqlite: schema: %w", err)
	}
	if err := ensureDynamicColumn(write); err != nil {
		write.Close()
		return nil, err
	}
	read := write
	if !isMemory(path) {
		// Opened after the pragmas (WAL is a property of the database
		// file); each read connection gets a busy timeout via the DSN.
		readDSN := path + "?_pragma=busy_timeout(5000)"
		if strings.Contains(path, "?") {
			readDSN = path + "&_pragma=busy_timeout(5000)"
		}
		read, err = sql.Open(driverName, readDSN)
		if err != nil {
			write.Close()
			return nil, fmt.Errorf("sqlite: open read path: %w", err)
		}
		read.SetMaxOpenConns(4)
		read.SetMaxIdleConns(4)
	}
	upsert, err := write.Prepare(upsertSQL)
	if err != nil {
		read.Close()
		write.Close()
		return nil, fmt.Errorf("sqlite: prepare upsert: %w", err)
	}
	return &ReportRepository{write: write, read: read, upsert: upsert}, nil
}

// isMemory reports whether path names an in-memory database (MemoryPath or
// a mode=memory DSN): WAL does not apply there and each connection would be
// a separate database (ADR 0009).
func isMemory(path string) bool {
	return path == MemoryPath || strings.Contains(path, "mode=memory")
}

const schema = `
CREATE TABLE IF NOT EXISTS reports (
	rule       TEXT    NOT NULL,
	target     TEXT    NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	dynamic    INTEGER NOT NULL DEFAULT 0,
	body       TEXT    NOT NULL,
	PRIMARY KEY (rule, target)
);
CREATE INDEX IF NOT EXISTS idx_reports_rule ON reports (rule);
CREATE INDEX IF NOT EXISTS idx_reports_target ON reports (target);
CREATE INDEX IF NOT EXISTS idx_reports_expires_at ON reports (expires_at);
`

// ensureDynamicColumn adds the dynamic column to a pre-upgrade table
// with one guarded ALTER TABLE at open: the schema is create-if-not-
// exists only (no migration runner), and old rows are fixed reports by
// construction, so the 0 default is exact, not a guess (spec:
// Persistence).
func ensureDynamicColumn(db *sql.DB) error {
	var present int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('reports') WHERE name = 'dynamic'`).Scan(&present); err != nil {
		return fmt.Errorf("sqlite: schema: %w", err)
	}
	if present > 0 {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE reports ADD COLUMN dynamic INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("sqlite: schema: add dynamic column: %w", err)
	}
	return nil
}

// body is the JSON record body: everything but the indexed columns.
// Sample carries the Event whole, rendered by the same wire form as
// the report document's sample (one wire form for serve and store).
// Dynamic flags the dynamic report (spec: Persistence); the indexed
// dynamic column is the query lever, the body flag is the record's own.
type body struct {
	Chain   string                `json:"chain"`
	Reason  string                `json:"reason"`
	Tracker domain.TrackerReading `json:"tracker"`
	Sample  domain.Event          `json:"sample"`
	Penalty *domain.Penalty       `json:"penalty,omitempty"`
	Dynamic bool                  `json:"dynamic,omitempty"`
}

func (r *ReportRepository) Close() error {
	err := r.read.Close()
	if r.read != r.write {
		if werr := r.write.Close(); err == nil {
			err = werr
		}
	}
	return err
}

// Upsert inserts or overwrites the report for (rule, target) in one prepared
// statement; created_at is fixed at first insert by the statement, no pre-read.
// A dynamic report stores the 0 sentinel in expires_at (its NOT NULL cannot
// be relaxed in place, spec: Persistence).
func (r *ReportRepository) Upsert(ctx context.Context, rec domain.Report) error {
	rawBody, err := json.Marshal(body{
		Chain:   rec.Chain,
		Reason:  rec.Reason,
		Tracker: rec.Tracker,
		Sample:  rec.Sample,
		Penalty: rec.Penalty,
		Dynamic: rec.Dynamic,
	})
	if err != nil {
		return fmt.Errorf("sqlite: upsert: %w", err)
	}
	expiresNS := rec.ExpiresAt.UnixNano()
	if rec.Dynamic {
		expiresNS = 0
	}
	_, err = r.upsert.ExecContext(ctx,
		rec.Rule, rec.Target,
		rec.CreatedAt.UnixNano(), rec.UpdatedAt.UnixNano(), expiresNS, rec.Dynamic,
		string(rawBody))
	if err != nil {
		return fmt.Errorf("sqlite: upsert: %w", err)
	}
	return nil
}

// List returns matching, live reports sorted by created_at descending
// (ties by rule, target), on the read path's own connections (ADR 0009):
// unexpired fixed rows plus every dynamic row (spec: Persistence - a
// dynamic row's 0 sentinel must not read as expired).
func (r *ReportRepository) List(ctx context.Context, q report.Query) ([]domain.Report, error) {
	query := `SELECT rule, target, created_at, updated_at, expires_at, body FROM reports WHERE (expires_at > ? OR dynamic = 1)`
	args := []any{q.Now.UnixNano()}
	if q.Rule != "" {
		query += ` AND rule = ?`
		args = append(args, q.Rule)
	}
	if q.Target != "" {
		query += ` AND target = ?`
		args = append(args, q.Target)
	}
	query += ` ORDER BY created_at DESC, rule ASC, target ASC`

	rows, err := r.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list: %w", err)
	}
	defer rows.Close()

	var out []domain.Report
	for rows.Next() {
		var (
			rec       domain.Report
			createdNS int64
			updatedNS int64
			expiresNS int64
			rawBody   string
		)
		if err := rows.Scan(&rec.Rule, &rec.Target, &createdNS, &updatedNS, &expiresNS, &rawBody); err != nil {
			return nil, fmt.Errorf("sqlite: list: %w", err)
		}
		var b body
		if err := json.Unmarshal([]byte(rawBody), &b); err != nil {
			return nil, fmt.Errorf("sqlite: list: %w", err)
		}
		rec.Chain = b.Chain
		rec.Reason = b.Reason
		rec.Tracker = b.Tracker
		rec.Sample = b.Sample
		rec.Penalty = b.Penalty
		rec.Dynamic = b.Dynamic
		rec.CreatedAt = time.Unix(0, createdNS).UTC()
		rec.UpdatedAt = time.Unix(0, updatedNS).UTC()
		if b.Dynamic {
			// The sentinel reads back as the absent expiry.
			rec.ExpiresAt = time.Time{}
		} else {
			rec.ExpiresAt = time.Unix(0, expiresNS).UTC()
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list: %w", err)
	}
	return out, nil
}

// Delete removes matching reports regardless of expiry and returns the count;
// at least one filter is required (ErrUnfiltered): deletion is never delete-all.
func (r *ReportRepository) Delete(ctx context.Context, q report.Query) (int, error) {
	var (
		query string
		args  []any
	)
	switch {
	case q.Rule != "" && q.Target != "":
		query = `DELETE FROM reports WHERE rule = ? AND target = ?`
		args = []any{q.Rule, q.Target}
	case q.Rule != "":
		query = `DELETE FROM reports WHERE rule = ?`
		args = []any{q.Rule}
	case q.Target != "":
		query = `DELETE FROM reports WHERE target = ?`
		args = []any{q.Target}
	default:
		return 0, fmt.Errorf("sqlite: delete: %w", report.ErrUnfiltered)
	}
	res, err := r.write.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: delete: %w", err)
	}
	return int(n), nil
}

// DeleteExpired deletes fixed rows whose expires_at has passed and
// returns the count; dynamic rows carry the 0 sentinel and die by
// value, never by time - the sweep must not eat them (spec:
// Persistence).
func (r *ReportRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	res, err := r.write.ExecContext(ctx, `DELETE FROM reports WHERE expires_at <= ? AND dynamic = 0`, now.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("sqlite: delete expired: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: delete expired: %w", err)
	}
	return int(n), nil
}

// DeleteDynamic deletes every dynamic row and returns the count - the
// startup sweep (spec: Startup sweep); fixed rows are never touched.
func (r *ReportRepository) DeleteDynamic(ctx context.Context) (int, error) {
	res, err := r.write.ExecContext(ctx, `DELETE FROM reports WHERE dynamic = 1`)
	if err != nil {
		return 0, fmt.Errorf("sqlite: delete dynamic: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: delete dynamic: %w", err)
	}
	return int(n), nil
}
