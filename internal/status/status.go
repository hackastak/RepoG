package status

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/hackastak/repog/internal/github"
)

// Result contains all status information.
type Result struct {
	Repos struct {
		Total         int `json:"total"`
		Owned         int `json:"owned"`
		Starred       int `json:"starred"`
		EmbeddedCount int `json:"embeddedCount"`
		PendingEmbed  int `json:"pendingEmbed"`
	} `json:"repos"`
	Sync struct {
		LastSyncedAt   *string `json:"lastSyncedAt"`
		LastSyncStatus *string `json:"lastSyncStatus"`
	} `json:"sync"`
	Embed struct {
		LastEmbeddedAt  *string `json:"lastEmbeddedAt"`
		TotalChunks     int     `json:"totalChunks"`
		TotalEmbeddings int     `json:"totalEmbeddings"`
	} `json:"embed"`
	RateLimit *github.RateLimitInfo `json:"rateLimit"`
	DB        struct {
		Path      string `json:"path"`
		SizeBytes int64  `json:"sizeBytes"`
		SizeMB    string `json:"sizeMb"`
	} `json:"db"`
	GeneratedAt string         `json:"generatedAt"`
	Languages   []LanguageStat `json:"languages"`
}

// LanguageStat is one row of the language breakdown over indexed repos.
type LanguageStat struct {
	Language string  `json:"language"`
	Count    int     `json:"count"`
	Percent  float64 `json:"percent"` // share of all repos, 0..100
}

// Collect gathers the local status snapshot from the database. It performs only
// SQLite queries plus a stat() of the DB file — no network — so it is fast and
// unit-testable. RateLimit is left nil for the caller to populate via
// FetchRateLimit. The embedded-state predicate (embedded_hash = pushed_at_hash)
// matches the rest of the app so every surface agrees on "embedded".
func Collect(ctx context.Context, database *sql.DB, dbPath string) (Result, error) {
	var r Result
	r.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	// Repo stats in one aggregate. SUM over an empty table yields NULL, so the
	// columns are scanned as NullInt64 (an empty DB reports zeros, not an error).
	var total, owned, starred, embedded, pending sql.NullInt64
	err := database.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			SUM(CASE WHEN is_owned = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN is_starred = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN embedded_hash IS NOT NULL AND embedded_hash = pushed_at_hash THEN 1 ELSE 0 END),
			SUM(CASE WHEN embedded_hash IS NULL OR embedded_hash != pushed_at_hash THEN 1 ELSE 0 END)
		FROM repos
	`).Scan(&total, &owned, &starred, &embedded, &pending)
	if err != nil {
		return r, fmt.Errorf("collect repo stats: %w", err)
	}
	r.Repos.Total = int(total.Int64)
	r.Repos.Owned = int(owned.Int64)
	r.Repos.Starred = int(starred.Int64)
	r.Repos.EmbeddedCount = int(embedded.Int64)
	r.Repos.PendingEmbed = int(pending.Int64)

	// Sync state: prefer the sync_state log, fall back to repos.synced_at.
	var syncStatus, lastSynced sql.NullString
	_ = database.QueryRowContext(ctx, `
		SELECT status, last_synced_at FROM sync_state
		ORDER BY last_synced_at DESC LIMIT 1
	`).Scan(&syncStatus, &lastSynced)
	if !lastSynced.Valid {
		_ = database.QueryRowContext(ctx, "SELECT MAX(synced_at) FROM repos").Scan(&lastSynced)
		if lastSynced.Valid {
			syncStatus = sql.NullString{String: "completed", Valid: true}
		}
	}
	if lastSynced.Valid {
		r.Sync.LastSyncedAt = &lastSynced.String
	}
	if syncStatus.Valid {
		r.Sync.LastSyncStatus = &syncStatus.String
	}

	// Knowledge base counts.
	_ = database.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks").Scan(&r.Embed.TotalChunks)
	_ = database.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunk_embeddings").Scan(&r.Embed.TotalEmbeddings)

	// Last embed timestamp.
	var lastEmbedded sql.NullString
	_ = database.QueryRowContext(ctx, "SELECT MAX(embedded_at) FROM repos WHERE embedded_at IS NOT NULL").Scan(&lastEmbedded)
	if lastEmbedded.Valid {
		r.Embed.LastEmbeddedAt = &lastEmbedded.String
	}

	// Database file stats.
	r.DB.Path = dbPath
	if info, statErr := os.Stat(dbPath); statErr == nil {
		r.DB.SizeBytes = info.Size()
		r.DB.SizeMB = fmt.Sprintf("%.2f MB", float64(info.Size())/(1024*1024))
	}

	// Language breakdown over indexed repos.
	langs, err := collectLanguages(ctx, database, r.Repos.Total)
	if err != nil {
		return r, fmt.Errorf("collect languages: %w", err)
	}
	r.Languages = langs

	return r, nil
}

// collectLanguages returns per-language repo counts (with each language's share
// of the total), most-common first. NULL/empty languages bucket as "Unknown".
// Unlike the single-row QueryRow scans above, this walks a multi-row result set:
// QueryContext -> for rows.Next() -> rows.Scan() -> rows.Err().
func collectLanguages(ctx context.Context, database *sql.DB, total int) ([]LanguageStat, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(language, ''), 'Unknown') AS lang, COUNT(*) AS n
		FROM repos
		GROUP BY lang
		ORDER BY n DESC, lang ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []LanguageStat
	for rows.Next() {
		var s LanguageStat
		if err := rows.Scan(&s.Language, &s.Count); err != nil {
			return nil, err
		}
		if total > 0 {
			s.Percent = float64(s.Count) / float64(total) * 100
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FetchRateLimit retrieves the GitHub rate limit using the given PAT. It returns
// nil when the PAT is empty or the call fails, matching the CLI's "unavailable"
// behavior. Kept separate from Collect so the snapshot stays network-free.
func FetchRateLimit(ctx context.Context, pat string) *github.RateLimitInfo {
	if pat == "" {
		return nil
	}
	client := github.NewClient(pat)
	return github.GetRateLimitInfo(ctx, client)
}
