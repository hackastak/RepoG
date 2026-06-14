package tui

import (
	"database/sql"

	tea "github.com/charmbracelet/bubbletea"
)

// repoRow is the presentation-shaped view of a repos table row. It carries only
// what the Repos table renders; richer per-repo data is loaded on demand when a
// repo is opened (recommend/summarize), so this stays cheap for large lists.
type repoRow struct {
	FullName  string
	Language  string
	Stars     int
	IsOwned   bool
	IsStarred bool
	Synced    bool // a sync has populated this repo (synced_at IS NOT NULL)
	Embedded  bool // embeddings are current (embedded_hash == pushed_at_hash)
}

// reposLoadedMsg is delivered when the async repo load finishes.
type reposLoadedMsg struct {
	rows []repoRow
	err  error
}

// loadReposCmd queries the repos table off the UI goroutine and returns the
// result as a tea.Msg. The "embedded" derivation mirrors commands/status.go
// (embedded_hash IS NOT NULL AND embedded_hash = pushed_at_hash) so the TUI and
// the CLI report the same state — no duplicated business rule, just the same
// SQL predicate.
func loadReposCmd(database *sql.DB) tea.Cmd {
	return func() tea.Msg {
		if database == nil {
			return reposLoadedMsg{} // not configured yet; empty list, no error
		}
		rows, err := database.Query(`
			SELECT
				full_name,
				COALESCE(language, ''),
				stars,
				is_owned,
				is_starred,
				CASE WHEN synced_at IS NOT NULL THEN 1 ELSE 0 END,
				CASE WHEN embedded_hash IS NOT NULL AND embedded_hash = pushed_at_hash THEN 1 ELSE 0 END
			FROM repos
			ORDER BY stars DESC, full_name ASC
		`)
		if err != nil {
			return reposLoadedMsg{err: err}
		}
		defer rows.Close()

		var out []repoRow
		for rows.Next() {
			var r repoRow
			if err := rows.Scan(
				&r.FullName, &r.Language, &r.Stars,
				&r.IsOwned, &r.IsStarred, &r.Synced, &r.Embedded,
			); err != nil {
				return reposLoadedMsg{err: err}
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return reposLoadedMsg{err: err}
		}
		return reposLoadedMsg{rows: out}
	}
}
