package status

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/hackastak/repog/internal/db"
)

// newTestDB opens a fresh, migrated SQLite database in a temp dir and returns
// it alongside its path. Cleanup is registered automatically.
func newTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(path, 768)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database, path
}

// nullable maps an empty string to a SQL NULL so we can exercise the
// NULL/empty-language bucketing.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestCollectEmptyDB confirms a freshly migrated DB reports zeros (not an error)
// — this is the NullInt64 path: SUM over no rows is NULL.
func TestCollectEmptyDB(t *testing.T) {
	database, path := newTestDB(t)

	res, err := Collect(context.Background(), database, path)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if res.Repos.Total != 0 {
		t.Errorf("Total = %d, want 0", res.Repos.Total)
	}
	if res.Repos.PendingEmbed != 0 {
		t.Errorf("PendingEmbed = %d, want 0", res.Repos.PendingEmbed)
	}
	if len(res.Languages) != 0 {
		t.Errorf("Languages = %d entries, want 0", len(res.Languages))
	}
	if res.RateLimit != nil {
		t.Error("RateLimit should be nil (Collect makes no network call)")
	}
}

// TestCollectCountsAndLanguages inserts a known set of repos and checks the
// aggregate counts, the embedded predicate, and the language breakdown.
func TestCollectCountsAndLanguages(t *testing.T) {
	database, path := newTestDB(t)
	ctx := context.Background()

	repos := []struct {
		id           int
		fullName     string
		language     string
		stars        int
		owned        bool
		starred      bool
		embeddedHash string
		pushedHash   string
	}{
		{1, "me/go-a", "Go", 100, true, true, "h1", "h1"},    // embedded (hashes match)
		{2, "me/go-b", "Go", 50, true, false, "", "h2"},      // pending (no embed hash)
		{3, "other/rust", "Rust", 10, false, true, "", "h3"}, // pending, starred
		{4, "other/blank", "", 5, false, false, "", "h4"},    // NULL language -> Unknown
	}
	for _, r := range repos {
		_, err := database.ExecContext(ctx, `
			INSERT INTO repos (github_id, owner, name, full_name, language, stars,
				is_owned, is_starred, embedded_hash, pushed_at_hash, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			r.id, "owner", "name", r.fullName, nullable(r.language), r.stars,
			boolToInt(r.owned), boolToInt(r.starred), nullable(r.embeddedHash),
			r.pushedHash, "2026-06-10T00:00:00Z",
		)
		if err != nil {
			t.Fatalf("insert repo %d: %v", r.id, err)
		}
	}

	// One chunk so TotalChunks is non-zero.
	if _, err := database.ExecContext(ctx,
		"INSERT INTO chunks (repo_id, chunk_type, content) VALUES (1, 'metadata', '{}')"); err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	res, err := Collect(ctx, database, path)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Aggregate counts.
	checks := []struct {
		name string
		got  int
		want int
	}{
		{"Total", res.Repos.Total, 4},
		{"Owned", res.Repos.Owned, 2},
		{"Starred", res.Repos.Starred, 2},
		{"Embedded", res.Repos.EmbeddedCount, 1},
		{"PendingEmbed", res.Repos.PendingEmbed, 3},
		{"TotalChunks", res.Embed.TotalChunks, 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}

	// Language breakdown: Go=2 (50%), then Rust=1 and Unknown=1 (25% each),
	// ordered by count desc, then language asc (so Rust precedes Unknown).
	if len(res.Languages) != 3 {
		t.Fatalf("Languages = %d entries, want 3: %+v", len(res.Languages), res.Languages)
	}
	if got := res.Languages[0]; got.Language != "Go" || got.Count != 2 || got.Percent != 50.0 {
		t.Errorf("Languages[0] = %+v, want {Go 2 50}", got)
	}
	if got := res.Languages[1].Language; got != "Rust" {
		t.Errorf("Languages[1] = %q, want Rust", got)
	}
	if got := res.Languages[2].Language; got != "Unknown" {
		t.Errorf("Languages[2] = %q, want Unknown", got)
	}
}
