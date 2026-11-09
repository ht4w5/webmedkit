package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/domain/report"
)

// Schema contract: one row per (rule, target) with indexed rule, target and
// expiry columns, so ?target=-only queries never full-scan the table.

func openTestRepo(t *testing.T) *ReportRepository {
	t.Helper()
	repo, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

// TestConcurrentUpsertsAreDeterministic: one connection serializes concurrent
// writes (ADR 0007) - every upsert lands, duplicates refresh the same row.
func TestConcurrentUpsertsAreDeterministic(t *testing.T) {
	for _, path := range []string{":memory:", filepath.Join(t.TempDir(), "reports.db")} {
		t.Run(path, func(t *testing.T) {
			repo, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer repo.Close()
			base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)

			// 8 writers x 8 upserts; even writers share one
			// (rule, target), odd writers have one distinct target each.
			const writers, perWriter = 8, 8
			var wg sync.WaitGroup
			errs := make(chan error, writers*perWriter)
			for w := range writers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := range perWriter {
						target := fmt.Sprintf("203.0.113.%d/32", i)
						if w%2 == 0 {
							target = "198.51.100.7/32"
						}
						rec := domain.Report{
							Rule: "r", Chain: "c", Reason: "why", Target: target,
							CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
						}
						if err := repo.Upsert(context.Background(), rec); err != nil {
							errs <- err
						}
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Errorf("Upsert error: %v", err)
			}

			// The shared row plus the 8 distinct ones, all visible.
			got, err := repo.List(context.Background(), report.Query{Now: base})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != 9 {
				t.Errorf("rows = %d, want 9 (one per (rule, target))", len(got))
			}
		})
	}
}

// singleColumnIndexes returns the table's single-column index names by the
// column they cover (PRAGMA index_list/index_info).
func singleColumnIndexes(t *testing.T, repo *ReportRepository) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	rows, err := repo.read.Query(`SELECT name FROM pragma_index_list('reports')`)
	if err != nil {
		t.Fatalf("pragma_index_list: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		cols, err := repo.read.Query(`SELECT name FROM pragma_index_info(?)`, name)
		if err != nil {
			t.Fatalf("pragma_index_info(%q): %v", name, err)
		}
		var indexCols []string
		for cols.Next() {
			var col string
			if err := cols.Scan(&col); err != nil {
				cols.Close()
				t.Fatal(err)
			}
			indexCols = append(indexCols, col)
		}
		cols.Close()
		if err := cols.Err(); err != nil {
			t.Fatal(err)
		}
		if len(indexCols) == 1 {
			out[indexCols[0]] = true
		}
	}
	return out
}

// TestSchemaHasStandaloneIndexesOnRuleTargetAndExpiry: standalone indexes on
// rule, target (the composite PK alone serves only rule-prefix lookups) and expiry.
func TestSchemaHasStandaloneIndexesOnRuleTargetAndExpiry(t *testing.T) {
	repo := openTestRepo(t)
	indexed := singleColumnIndexes(t, repo)
	for _, col := range []string{"rule", "target", "expires_at"} {
		if !indexed[col] {
			t.Errorf("no standalone index on column %q (indexes: %v)", col, indexed)
		}
	}
}

// queryPlan returns the detail column of EXPLAIN QUERY PLAN for one statement.
func queryPlan(t *testing.T, repo *ReportRepository, query string, args ...any) []string {
	t.Helper()
	rows, err := repo.read.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN %s: %v", query, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		out = append(out, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTargetOnlyQueriesDoNotFullScan: target-only List and Delete run as
// index searches on the target column, never a full scan.
func TestTargetOnlyQueriesDoNotFullScan(t *testing.T) {
	repo := openTestRepo(t)
	for _, tt := range []struct {
		name  string
		query string
		args  []any
	}{
		{"list", `SELECT rule, target, created_at, updated_at, expires_at, body FROM reports WHERE (expires_at > ? OR dynamic = 1) AND target = ?`, []any{0, ""}},
		{"delete", `DELETE FROM reports WHERE target = ?`, []any{""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plan := queryPlan(t, repo, tt.query, tt.args...)
			searched := false
			for _, detail := range plan {
				if detail == "SCAN reports" {
					t.Errorf("target-only query full-scans the table: %v", plan)
				}
				if strings.Contains(detail, "SEARCH reports USING INDEX") && strings.Contains(detail, "target=?") {
					searched = true
				}
			}
			if !searched {
				t.Errorf("target-only query does not search the target index: %v", plan)
			}
		})
	}
}

// TestTargetOnlyListAndDelete: target-only delete and list stay correct
// with the indexes in place.
func TestTargetOnlyListAndDelete(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	rec := domain.Report{
		Rule: "r", Chain: "c", Reason: "why", Target: "203.0.113.0/24",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
	}
	other := rec
	other.Rule = "r2"
	other.Target = "198.51.100.0/24"
	for _, r := range []domain.Report{rec, other} {
		if err := repo.Upsert(ctx, r); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	got, err := repo.List(ctx, report.Query{Target: rec.Target, Now: base})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Target != rec.Target {
		t.Errorf("List by target = %+v, want only %q", got, rec.Target)
	}

	n, err := repo.Delete(ctx, report.Query{Target: rec.Target})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n != 1 {
		t.Errorf("Delete by target = %d, want 1", n)
	}
}

// TestFileDatabaseRunsInWALSynchronousNormal: file databases open in
// journal_mode=WAL with synchronous=NORMAL (commits survive process crashes).
func TestFileDatabaseRunsInWALSynchronousNormal(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	var mode string
	if err := repo.write.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
	var syncMode int64
	if err := repo.write.QueryRow(`PRAGMA synchronous`).Scan(&syncMode); err != nil {
		t.Fatalf("synchronous: %v", err)
	}
	if syncMode != 1 {
		t.Errorf("synchronous = %d, want 1 (NORMAL)", syncMode)
	}
}

// TestWrittenReportSurvivesReopen: a written report survives reopening the
// database (WAL + synchronous=NORMAL), keeping its original created_at and
// its penalty.
func TestWrittenReportSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports.db")
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	rate := int64(1000000)
	rec := domain.Report{
		Rule: "r", Chain: "c", Reason: "why", Target: "203.0.113.7/32",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
		Sample: domain.Event{
			Time: base, RemoteAddr: netip.MustParseAddr("203.0.113.7"), Method: "GET",
			Path: "/releases/24.04.iso", Agent: "Wget/1.21", Status: 200, SentBytes: 6000,
			ContentLength: 6000, ContentRangeTotal: 3000000,
		},
		Penalty: &domain.Penalty{Action: domain.PenaltyLimitBandwidth, Rate: &rate},
	}

	repo, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	got, err := reopened.List(ctx, report.Query{Now: base})
	if err != nil {
		t.Fatalf("List after reopen: %v", err)
	}
	if len(got) != 1 || got[0].CreatedAt != rec.CreatedAt {
		t.Errorf("rows after reopen = %+v, want the written report with created_at %v", got, rec.CreatedAt)
	}
	if p := got[0].Penalty; p == nil || p.Action != domain.PenaltyLimitBandwidth ||
		p.Rate == nil || *p.Rate != rate {
		t.Errorf("penalty after reopen = %+v, want limit-bandwidth at %d", got[0].Penalty, rate)
	}
	checkSample(t, got[0].Sample, rec.Sample)
}

// rawSampleMember returns the raw "sample" member of a JSON object, so a
// body blob and a document can be compared member for member.
func rawSampleMember(t *testing.T, doc string) json.RawMessage {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &obj); err != nil {
		t.Fatalf("unmarshal %q: %v", doc, err)
	}
	s, ok := obj["sample"]
	if !ok {
		t.Fatalf("%q has no sample member", doc)
	}
	return s
}

// TestBodyAndDocumentRenderTheSampleIdentically: the report body a
// Trigger persists and the report document the daemon serves draw the
// sample from one wire form - byte for byte - so storage and serving
// cannot disagree about what a sample is (ADR 0021).
func TestBodyAndDocumentRenderTheSampleIdentically(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	rec := domain.Report{
		Rule: "r", Chain: "c", Reason: "why", Target: "203.0.113.7/32",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
		// The whole Event is the sample; every field - the content
		// observations included - is part of the contract (ticket 02).
		Sample: domain.Event{
			Time:              base,
			RemoteAddr:        netip.MustParseAddr("203.0.113.7"),
			Method:            "GET",
			Path:              "/releases/24.04.iso",
			Agent:             "Wget/1.21",
			Status:            200,
			SentBytes:         6000,
			ContentLength:     6000,
			ContentRangeTotal: 3000000,
		},
	}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// The persisted body's sample, straight from the body blob.
	var rawBody string
	if err := repo.read.QueryRow(
		`SELECT body FROM reports WHERE rule = ? AND target = ?`, rec.Rule, rec.Target,
	).Scan(&rawBody); err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodySample := rawSampleMember(t, rawBody)

	// The served document's sample, straight from the report entry.
	doc, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal document entry: %v", err)
	}
	docSample := rawSampleMember(t, string(doc))

	want := []byte(`{"time":"2025-10-17T18:00:00Z","remote_addr":"203.0.113.7","method":"GET","path":"/releases/24.04.iso","agent":"Wget/1.21","status":200,"sent_bytes":6000,"content_length":6000,"content_range_total":3000000,"size_bytes":0}`)
	if !bytes.Equal(bodySample, docSample) {
		t.Errorf("body sample = %s\ndocument sample = %s\nwant one wire form", bodySample, docSample)
	}
	if !bytes.Equal(bodySample, want) {
		t.Errorf("body sample = %s\nwant %s", bodySample, want)
	}

	// The sample survives the body blob: List hands the document the
	// same sample the Trigger persisted.
	got, err := repo.List(ctx, report.Query{Now: base})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %+v, want one", got)
	}
	checkSample(t, got[0].Sample, rec.Sample)
}

// TestPreReshapeBodyKeepsServing: a report body written before the
// sample reshape - the pre-feature six-key sample - keeps serving
// through List after a daemon upgrade: the renamed and added sample
// fields read zero (zero means absent; the old keys are never
// dual-read), while the surviving keys keep their values.
func TestPreReshapeBodyKeepsServing(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)

	// The row is hand-written: no current code path can emit the
	// pre-reshape shape any more. The sample carries the pre-feature
	// six keys only.
	const legacyBody = `{"chain":"c","reason":"why","tracker":{"track":"bandwidth","value":11000,"leak":1000,"volume":10000},"sample":{"time":"2025-10-17T18:00:00Z","path":"/releases/24.04.iso","agent":"Wget/1.21","status":200,"sent":6000,"size":3000000},"penalty":{"action":"limit-bandwidth","rate":1000000}}`
	if _, err := repo.write.ExecContext(ctx,
		`INSERT INTO reports (rule, target, created_at, updated_at, expires_at, body) VALUES (?, ?, ?, ?, ?, ?)`,
		"r", "203.0.113.7/32", base.UnixNano(), base.UnixNano(), base.Add(time.Hour).UnixNano(), legacyBody,
	); err != nil {
		t.Fatalf("insert pre-reshape row: %v", err)
	}

	got, err := repo.List(ctx, report.Query{Now: base})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %+v, want the one pre-reshape row", got)
	}
	rec := got[0]
	if rec.Rule != "r" || rec.Target != "203.0.113.7/32" || rec.Chain != "c" || rec.Reason != "why" {
		t.Errorf("record = %+v, want the pre-reshape row's identity kept", rec)
	}
	if p := rec.Penalty; p == nil || p.Action != domain.PenaltyLimitBandwidth || p.Rate == nil || *p.Rate != 1000000 {
		t.Errorf("penalty = %+v, want the pre-reshape row's penalty kept", rec.Penalty)
	}
	// The surviving keys keep their values; the renamed and added fields
	// read zero (zero means absent).
	checkSample(t, rec.Sample, domain.Event{
		Time: base, Path: "/releases/24.04.iso", Agent: "Wget/1.21", Status: 200,
	})
}

// checkSample reports every sample field that differs from want as
// "sample <field> = ..., want ..." - the field-by-field comparison
// behind the sample assertions.
func checkSample(t *testing.T, got, want domain.Event) {
	t.Helper()
	if !got.Time.Equal(want.Time) {
		t.Errorf("sample time = %v, want %v", got.Time, want.Time)
	}
	for _, f := range []struct {
		name      string
		got, want any
	}{
		{"remote_addr", got.RemoteAddr, want.RemoteAddr},
		{"method", got.Method, want.Method},
		{"path", got.Path, want.Path},
		{"agent", got.Agent, want.Agent},
		{"status", got.Status, want.Status},
		{"sent_bytes", got.SentBytes, want.SentBytes},
		{"content_length", got.ContentLength, want.ContentLength},
		{"content_range_total", got.ContentRangeTotal, want.ContentRangeTotal},
		{"size_bytes", got.SizeBytes, want.SizeBytes},
	} {
		if f.got != f.want {
			t.Errorf("sample %s = %v, want %v", f.name, f.got, f.want)
		}
	}
}

// TestQueryDoesNotWaitBehindInflightWrite: holding the sole write connection
// never blocks List (the read path has its own connections).
func TestQueryDoesNotWaitBehindInflightWrite(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	rec := domain.Report{
		Rule: "r", Chain: "c", Reason: "why", Target: "203.0.113.7/32",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
	}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// The in-flight write holds the write connection.
	conn, err := repo.write.Conn(ctx)
	if err != nil {
		t.Fatalf("write conn: %v", err)
	}
	defer conn.Close()

	type result struct {
		reports []domain.Report
		err     error
	}
	done := make(chan result, 1)
	go func() {
		got, err := repo.List(ctx, report.Query{Now: base})
		done <- result{got, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("List: %v", r.err)
		}
		if len(r.reports) != 1 {
			t.Errorf("List during the in-flight write = %+v, want the committed row", r.reports)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("List waited for the in-flight write")
	}
}

// TestWriteDoesNotWaitBehindInflightQuery: a query holding its connection
// never blocks Upsert on the single write connection.
func TestWriteDoesNotWaitBehindInflightQuery(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	rec := domain.Report{
		Rule: "r", Chain: "c", Reason: "why", Target: "203.0.113.7/32",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
	}

	// The in-flight query holds the read path's connection.
	conn, err := repo.read.Conn(ctx)
	if err != nil {
		t.Fatalf("read conn: %v", err)
	}
	defer conn.Close()

	done := make(chan error, 1)
	go func() { done <- repo.Upsert(ctx, rec) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Upsert waited for the in-flight query")
	}

	got, err := repo.List(ctx, report.Query{Now: base})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("rows after the concurrent write = %+v, want the upserted report", got)
	}
}

// TestMemoryDatabaseWorksAcrossSplitPaths: :memory: cannot split connections,
// so the read path falls back to the single write connection (ADR 0009).
func TestMemoryDatabaseWorksAcrossSplitPaths(t *testing.T) {
	repo := openTestRepo(t)
	if repo.read != repo.write {
		t.Fatalf(":memory: read path = %p, want the write-connection fallback %p", repo.read, repo.write)
	}
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	rec := domain.Report{
		Rule: "r", Chain: "c", Reason: "why", Target: "203.0.113.7/32",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
	}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := repo.List(ctx, report.Query{Now: base})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Target != rec.Target {
		t.Errorf("rows = %+v, want the upserted report", got)
	}
}

// TestUpsertPreservesCreatedAtOnUpdate: a later Trigger overwrites updated_at,
// expiry, tracker, sample and penalty but never the first-trigger created_at
// (no pre-read).
func TestUpsertPreservesCreatedAtOnUpdate(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	first := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	rec := domain.Report{
		Rule: "r", Chain: "c", Reason: "why", Target: "203.0.113.7/32",
		CreatedAt: first, UpdatedAt: first, ExpiresAt: first.Add(24 * time.Hour),
		Penalty: &domain.Penalty{Action: domain.PenaltyReject},
	}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// The update arrives with a different CreatedAt and penalty; the row's
	// first-trigger timestamp wins, the body is replaced.
	rec.CreatedAt = second
	rec.UpdatedAt = second
	rec.ExpiresAt = second.Add(24 * time.Hour)
	rec.Penalty = &domain.Penalty{Action: domain.PenaltyDrop}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	got, err := repo.List(ctx, report.Query{Now: second})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %+v, want one", got)
	}
	if got[0].CreatedAt != first {
		t.Errorf("created_at = %v, want the first-trigger %v", got[0].CreatedAt, first)
	}
	if got[0].UpdatedAt != second {
		t.Errorf("updated_at = %v, want the update %v", got[0].UpdatedAt, second)
	}
	if p := got[0].Penalty; p == nil || p.Action != domain.PenaltyDrop {
		t.Errorf("penalty = %+v, want the update's drop", got[0].Penalty)
	}
}

// --- dynamic reports (spec: Persistence) ---

// dynamicReport builds a dynamic report over base: no scheduled expiry
// (the 0 sentinel), the flag carried in the body.
func dynamicReport(base time.Time) domain.Report {
	return domain.Report{
		Rule: "dyn", Chain: "c", Reason: "why", Target: "203.0.113.7/32",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: time.Time{}, Dynamic: true,
		Tracker: domain.TrackerReading{Track: "bandwidth", Value: 11000, Leak: 1000, Volume: 10000},
	}
}

// TestDynamicRowsServeRegardlessOfExpiry: a dynamic row stores the 0
// sentinel in expires_at and the flag in its body, and the live-rows
// query serves it at any clock while an expired fixed row never serves.
func TestDynamicRowsServeRegardlessOfExpiry(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	dyn := dynamicReport(base)
	fixed := domain.Report{
		Rule: "fix", Chain: "c", Reason: "why", Target: "203.0.113.8/32",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
	}
	for _, rec := range []domain.Report{dyn, fixed} {
		if err := repo.Upsert(ctx, rec); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	// Well past the fixed row's expiry only the dynamic row serves,
	// carrying its flag and a zero (document-null) expiry.
	got, err := repo.List(ctx, report.Query{Now: base.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %+v, want only the dynamic row", got)
	}
	rec := got[0]
	if !rec.Dynamic {
		t.Errorf("dynamic = false, want the flag round-tripped")
	}
	if !rec.ExpiresAt.IsZero() {
		t.Errorf("expires_at = %v, want zero (the sentinel reads back absent)", rec.ExpiresAt)
	}
	if rec.Tracker.Value != 11000 {
		t.Errorf("tracker = %+v, want the stored reading", rec.Tracker)
	}

	// The sentinel is a column value, the flag is in the body.
	var expiresNS int64
	if err := repo.read.QueryRow(`SELECT expires_at FROM reports WHERE rule = ?`, dyn.Rule).Scan(&expiresNS); err != nil {
		t.Fatalf("read expires_at: %v", err)
	}
	if expiresNS != 0 {
		t.Errorf("expires_at column = %d, want the 0 sentinel", expiresNS)
	}
	var rawBody string
	if err := repo.read.QueryRow(`SELECT body FROM reports WHERE rule = ?`, dyn.Rule).Scan(&rawBody); err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(rawBody, `"dynamic":true`) {
		t.Errorf("body = %s, want the dynamic flag persisted", rawBody)
	}
}

// TestDeleteExpiredKeepsDynamicRows: the expiry sweep deletes expired
// fixed rows only - a dynamic row's 0 sentinel must never be eaten.
func TestDeleteExpiredKeepsDynamicRows(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)
	dyn := dynamicReport(base)
	expired := domain.Report{
		Rule: "fix", Chain: "c", Reason: "why", Target: "203.0.113.8/32",
		CreatedAt: base, UpdatedAt: base, ExpiresAt: base.Add(time.Hour),
	}
	for _, rec := range []domain.Report{dyn, expired} {
		if err := repo.Upsert(ctx, rec); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n, err := repo.DeleteExpired(ctx, base.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if n != 1 {
		t.Errorf("DeleteExpired = %d, want 1 (the expired fixed row only)", n)
	}
	got, err := repo.List(ctx, report.Query{Now: base.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || !got[0].Dynamic {
		t.Errorf("rows = %+v, want the dynamic row kept", got)
	}
}

// TestOldFormatDatabaseOpensAndServesAfterUpgrade: a pre-upgrade database
// (no dynamic column) opens with the column added by one guarded ALTER at
// open; its rows are fixed reports by construction and keep serving, and
// dynamic rows work over the upgraded table.
func TestOldFormatDatabaseOpensAndServesAfterUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports.db")
	ctx := context.Background()
	base := time.Date(2025, 10, 17, 18, 0, 0, 0, time.UTC)

	// The pre-upgrade schema and rows, hand-written: no current code
	// path emits the old shape (spec: Persistence, "old rows are fixed
	// reports by construction").
	const oldSchema = `
CREATE TABLE reports (
	rule       TEXT    NOT NULL,
	target     TEXT    NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	body       TEXT    NOT NULL,
	PRIMARY KEY (rule, target)
);
CREATE INDEX idx_reports_rule ON reports (rule);
CREATE INDEX idx_reports_target ON reports (target);
CREATE INDEX idx_reports_expires_at ON reports (expires_at);`
	const oldBody = `{"chain":"c","reason":"why","tracker":{"track":"bandwidth","value":11000,"leak":1000,"volume":10000},"sample":{"time":"2025-10-17T18:00:00Z","path":"/releases/24.04.iso","status":200},"penalty":{"action":"limit-bandwidth","rate":1000000}}`
	db, err := sql.Open(driverName, path)
	if err != nil {
		t.Fatalf("open old-format database: %v", err)
	}
	if _, err := db.Exec(oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	for _, row := range []struct {
		rule, target string
		expiresNS    int64
	}{
		{"r-live", "203.0.113.7/32", base.Add(time.Hour).UnixNano()},
		{"r-gone", "203.0.113.8/32", base.Add(-time.Hour).UnixNano()},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO reports (rule, target, created_at, updated_at, expires_at, body) VALUES (?, ?, ?, ?, ?, ?)`,
			row.rule, row.target, base.UnixNano(), base.UnixNano(), row.expiresNS, oldBody,
		); err != nil {
			t.Fatalf("insert old-format row: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close old-format database: %v", err)
	}

	repo, err := Open(path)
	if err != nil {
		t.Fatalf("Open over the old-format database: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	// The old rows serve as fixed reports: the default is exact.
	got, err := repo.List(ctx, report.Query{Now: base})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Rule != "r-live" || got[0].Dynamic {
		t.Errorf("rows = %+v, want the one live old-format row as a fixed report", got)
	}

	// New-format rows work over the upgraded table.
	dyn := dynamicReport(base)
	if err := repo.Upsert(ctx, dyn); err != nil {
		t.Fatalf("Upsert over the upgraded table: %v", err)
	}
	n, err := repo.DeleteExpired(ctx, base.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteExpired = %d, want 2 (both old-format rows expired by then; the dynamic row is never eaten)", n)
	}
	got, err = repo.List(ctx, report.Query{Now: base.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("List after upgrade: %v", err)
	}
	if len(got) != 1 || !got[0].Dynamic {
		t.Errorf("rows = %+v, want the dynamic row over the upgraded table", got)
	}
}
