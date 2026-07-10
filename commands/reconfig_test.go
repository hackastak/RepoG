package commands

import (
	"database/sql"
	"testing"

	"github.com/hackastak/repog/internal/db"
)

func TestClearEmbeddings(t *testing.T) {
	database := openTestDB(t, 768)

	// A repo previously embedded at 768 dims.
	_, err := database.Exec(
		`INSERT INTO repos (github_id, owner, name, full_name, embedded_hash, embedded_at)
		 VALUES (1, 'octocat', 'hello', 'octocat/hello', 'abc123', '2020-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}

	// Switch to a different dimensionality.
	if err := clearEmbeddings(database, 1536); err != nil {
		t.Fatalf("clearEmbeddings: %v", err)
	}

	// Embedding state on the repo must be reset so it re-embeds.
	var hash, at sql.NullString
	if err := database.QueryRow(
		`SELECT embedded_hash, embedded_at FROM repos WHERE github_id = 1`).Scan(&hash, &at); err != nil {
		t.Fatalf("query repo: %v", err)
	}
	if hash.Valid || at.Valid {
		t.Errorf("embedded_hash/embedded_at not cleared: hash=%v at=%v", hash, at)
	}

	// The stored dimensions must reflect the new value.
	dims, err := db.GetEmbeddingDimensions(database)
	if err != nil {
		t.Fatalf("GetEmbeddingDimensions: %v", err)
	}
	if dims != 1536 {
		t.Errorf("stored dimensions = %d, want 1536", dims)
	}

	// The recreated embeddings table must exist and be empty.
	if n := countRows(t, database, "chunk_embeddings"); n != 0 {
		t.Errorf("chunk_embeddings should be empty after clear, got %d rows", n)
	}
}

func TestClearChunks(t *testing.T) {
	database := openTestDB(t, 768)

	_, err := database.Exec(
		`INSERT INTO repos (github_id, owner, name, full_name, pushed_at_hash, embedded_hash)
		 VALUES (1, 'octocat', 'hello', 'octocat/hello', 'pah', 'eh')`)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	var repoID int64
	if err := database.QueryRow(`SELECT id FROM repos WHERE github_id = 1`).Scan(&repoID); err != nil {
		t.Fatalf("select repo id: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'readme', 'hello world')`, repoID); err != nil {
		t.Fatalf("insert chunk: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO sync_state (repo_id, status) VALUES (?, 'completed')`, repoID); err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}

	if err := clearChunks(database); err != nil {
		t.Fatalf("clearChunks: %v", err)
	}

	if n := countRows(t, database, "chunks"); n != 0 {
		t.Errorf("chunks should be empty, got %d", n)
	}
	if n := countRows(t, database, "sync_state"); n != 0 {
		t.Errorf("sync_state should be empty, got %d", n)
	}

	var pah, eh sql.NullString
	if err := database.QueryRow(
		`SELECT pushed_at_hash, embedded_hash FROM repos WHERE github_id = 1`).Scan(&pah, &eh); err != nil {
		t.Fatalf("query repo: %v", err)
	}
	if pah.Valid || eh.Valid {
		t.Errorf("repo sync/embed hashes not cleared: pushed_at_hash=%v embedded_hash=%v", pah, eh)
	}
}
