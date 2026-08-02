// Package sync handles repository ingestion from GitHub.
package sync

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hackastak/repog/internal/github"
)

// IngestOptions configures the ingestion pipeline.
type IngestOptions struct {
	IncludeOwned   bool
	IncludeStarred bool
	FullTree       bool
	MaxChunkSize   int // Maximum characters per chunk (calculated from embedding model token limit)
	DB             *sql.DB
	GitHubPAT      string
}

// IngestEvent represents a progress event from the ingestion pipeline.
type IngestEvent struct {
	Type    string // "repo", "skip", "error", "done"
	Repo    string
	Status  string // "new" or "updated" for type=="repo"
	Reason  string // for type=="skip" or "error"
	Total   int
	Skipped int
	Errors  int
}

// repoEntry holds a repo and its ownership flags.
type repoEntry struct {
	Repo      github.Repo
	IsOwned   bool
	IsStarred bool
}

// metadataChunk is the JSON structure for metadata chunks.
type metadataChunk struct {
	FullName      string   `json:"full_name"`
	Description   string   `json:"description"`
	Language      string   `json:"language"`
	Stars         int      `json:"stars"`
	Forks         int      `json:"forks"`
	Topics        []string `json:"topics"`
	Archived      bool     `json:"archived"`
	Fork          bool     `json:"fork"`
	DefaultBranch string   `json:"default_branch"`
	PushedAt      string   `json:"pushed_at"`
	HTMLURL       string   `json:"html_url"`
}

// hashPushedAt computes SHA-256 hash of the pushed_at timestamp.
func hashPushedAt(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}

// CalculateMaxCharsFromTokens calculates the maximum characters based on token limit.
// Uses a conservative ratio of 3 characters per token to stay safely under limits.
// This accounts for worst-case scenarios with special characters and encoding overhead.
func CalculateMaxCharsFromTokens(maxTokens int) int {
	if maxTokens <= 0 {
		return 25000 // Fallback to original default
	}
	// Use 90% of token limit to provide safety margin
	safeTokens := int(float64(maxTokens) * 0.90)
	// Conservative ratio: 3 chars per token
	// (Real-world average is ~4 chars/token, but we want to be safe)
	return safeTokens * 3
}

// splitContent splits content into chunks if it exceeds the max size.
// maxChars should be calculated using calculateMaxCharsFromTokens() based on the
// embedding provider's token limit. Returns a slice of content chunks.
func splitContent(content string, maxChars int) []string {
	if maxChars <= 0 {
		maxChars = 25000
	}

	if len(content) <= maxChars {
		return []string{content}
	}

	var chunks []string
	for i := 0; i < len(content); i += maxChars {
		end := i + maxChars
		if end > len(content) {
			end = len(content)
		}
		chunks = append(chunks, content[i:end])
	}

	return chunks
}

// IngestRepos runs the full ingestion pipeline.
// Events are sent to the returned channel as repos are processed.
// The channel is closed when ingestion completes.
// Never panics — all errors are sent as IngestEvent{Type: "error"}.
func IngestRepos(ctx context.Context, opts IngestOptions) <-chan IngestEvent {
	eventCh := make(chan IngestEvent, 100)

	go func() {
		defer close(eventCh)

		client := github.NewClient(opts.GitHubPAT)
		reposToProcess := make(map[string]repoEntry)

		// Collect owned repos
		if opts.IncludeOwned {
			repoCh, errCh := github.FetchOwnedRepos(ctx, client)
			for repo := range repoCh {
				reposToProcess[repo.FullName] = repoEntry{
					Repo:    repo,
					IsOwned: true,
				}
			}
			if err := <-errCh; err != nil {
				eventCh <- IngestEvent{Type: "error", Reason: err.Error()}
				return
			}
		}

		// Collect starred repos
		if opts.IncludeStarred {
			repoCh, errCh := github.FetchStarredRepos(ctx, client)
			for repo := range repoCh {
				if existing, ok := reposToProcess[repo.FullName]; ok {
					existing.IsStarred = true
					reposToProcess[repo.FullName] = existing
				} else {
					reposToProcess[repo.FullName] = repoEntry{
						Repo:      repo,
						IsStarred: true,
					}
				}
			}
			if err := <-errCh; err != nil {
				eventCh <- IngestEvent{Type: "error", Reason: err.Error()}
				return
			}
		}

		var totalProcessed, skippedCount, errorCount int

		for fullName, entry := range reposToProcess {
			repo := entry.Repo

			// Compute hash
			pushedAtHash := hashPushedAt(repo.PushedAt)

			// Check existing
			var existingHash sql.NullString
			var existingID int64
			err := opts.DB.QueryRow(
				"SELECT id, pushed_at_hash FROM repos WHERE github_id = ?",
				repo.ID,
			).Scan(&existingID, &existingHash)

			isExisting := err == nil

			// Check if file_tree chunk exists when full-tree is requested
			hasFileTree := false
			if isExisting && opts.FullTree {
				var count int
				err := opts.DB.QueryRow(
					"SELECT COUNT(*) FROM chunks WHERE repo_id = ? AND (chunk_type = 'file_tree' OR chunk_type LIKE 'file_tree_part_%')",
					existingID,
				).Scan(&count)
				hasFileTree = err == nil && count > 0
			}

			// Skip if unchanged (but not if full-tree requested and file_tree is missing)
			if isExisting && existingHash.Valid && existingHash.String == pushedAtHash {
				if !opts.FullTree || hasFileTree {
					skippedCount++
					eventCh <- IngestEvent{Type: "skip", Repo: fullName, Reason: "unchanged"}
					continue
				}
			}

			// Fetch README
			readme := github.FetchReadme(ctx, client, repo.Owner.Login, repo.Name)

			// Fetch file tree if needed
			var fileTree string
			if opts.FullTree || len(readme) >= 100 {
				fileTree = github.FetchFileTree(ctx, client, repo.Owner.Login, repo.Name, repo.DefaultBranch)
			}

			// Write to DB in transaction
			tx, err := opts.DB.BeginTx(ctx, nil)
			if err != nil {
				errorCount++
				eventCh <- IngestEvent{Type: "error", Repo: fullName, Reason: err.Error()}
				continue
			}

			// Upsert repo
			description := ""
			if repo.Description != nil {
				description = *repo.Description
			}
			language := ""
			if repo.Language != nil {
				language = *repo.Language
			}
			topics, _ := json.Marshal(repo.Topics)
			syncedAt := time.Now().UTC().Format(time.RFC3339)

			isOwned := 0
			if entry.IsOwned {
				isOwned = 1
			}
			isStarred := 0
			if entry.IsStarred {
				isStarred = 1
			}
			isPrivate := 0
			if repo.Private {
				isPrivate = 1
			}
			isArchived := 0
			if repo.Archived {
				isArchived = 1
			}
			isFork := 0
			if repo.Fork {
				isFork = 1
			}

			result, err := tx.Exec(`
				INSERT INTO repos (
					github_id, owner, name, full_name, description, language,
					stars, forks, is_private, is_starred, is_owned, is_archived, is_fork,
					pushed_at, pushed_at_hash, topics, html_url, default_branch, synced_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(github_id) DO UPDATE SET
					owner = excluded.owner,
					name = excluded.name,
					full_name = excluded.full_name,
					description = excluded.description,
					language = excluded.language,
					stars = excluded.stars,
					forks = excluded.forks,
					is_private = excluded.is_private,
					is_starred = excluded.is_starred,
					is_owned = excluded.is_owned,
					is_archived = excluded.is_archived,
					is_fork = excluded.is_fork,
					pushed_at = excluded.pushed_at,
					pushed_at_hash = excluded.pushed_at_hash,
					topics = excluded.topics,
					html_url = excluded.html_url,
					default_branch = excluded.default_branch,
					synced_at = excluded.synced_at
			`,
				repo.ID, repo.Owner.Login, repo.Name, repo.FullName, description, language,
				repo.StargazersCount, repo.ForksCount, isPrivate, isStarred, isOwned, isArchived, isFork,
				repo.PushedAt, pushedAtHash, string(topics), repo.HTMLURL, repo.DefaultBranch, syncedAt,
			)
			if err != nil {
				_ = tx.Rollback()
				errorCount++
				eventCh <- IngestEvent{Type: "error", Repo: fullName, Reason: err.Error()}
				continue
			}

			// Get repo ID
			var repoID int64
			if isExisting {
				repoID = existingID
			} else {
				repoID, _ = result.LastInsertId()
			}

			// Delete existing chunks
			_, err = tx.Exec("DELETE FROM chunks WHERE repo_id = ?", repoID)
			if err != nil {
				_ = tx.Rollback()
				errorCount++
				eventCh <- IngestEvent{Type: "error", Repo: fullName, Reason: err.Error()}
				continue
			}

			// Insert metadata chunk
			metadata := metadataChunk{
				FullName:      repo.FullName,
				Description:   description,
				Language:      language,
				Stars:         repo.StargazersCount,
				Forks:         repo.ForksCount,
				Topics:        repo.Topics,
				Archived:      repo.Archived,
				Fork:          repo.Fork,
				DefaultBranch: repo.DefaultBranch,
				PushedAt:      repo.PushedAt,
				HTMLURL:       repo.HTMLURL,
			}
			metadataJSON, _ := json.Marshal(metadata)

			_, err = tx.Exec(
				"INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)",
				repoID, string(metadataJSON),
			)
			if err != nil {
				_ = tx.Rollback()
				errorCount++
				eventCh <- IngestEvent{Type: "error", Repo: fullName, Reason: err.Error()}
				continue
			}

			// Insert readme chunk(s) if present
			// Split into multiple chunks if too large
			if readme != "" {
				chunkSize := opts.MaxChunkSize
				if chunkSize <= 0 {
					chunkSize = 25000 // Fallback to default
				}
				readmeChunks := splitContent(readme, chunkSize)
				readmeSuccess := true
				for i, chunk := range readmeChunks {
					chunkType := "readme"
					if len(readmeChunks) > 1 {
						chunkType = fmt.Sprintf("readme_part_%d", i+1)
					}
					_, err = tx.Exec(
						"INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, ?, ?)",
						repoID, chunkType, chunk,
					)
					if err != nil {
						_ = tx.Rollback()
						errorCount++
						eventCh <- IngestEvent{Type: "error", Repo: fullName, Reason: err.Error()}
						readmeSuccess = false
						break
					}
				}
				if !readmeSuccess {
					continue
				}
			}

			// Insert file_tree chunk(s) if present
			// Split into multiple chunks if too large
			if fileTree != "" {
				chunkSize := opts.MaxChunkSize
				if chunkSize <= 0 {
					chunkSize = 25000 // Fallback to default
				}
				fileTreeChunks := splitContent(fileTree, chunkSize)
				fileTreeSuccess := true
				for i, chunk := range fileTreeChunks {
					chunkType := "file_tree"
					if len(fileTreeChunks) > 1 {
						chunkType = fmt.Sprintf("file_tree_part_%d", i+1)
					}
					_, err = tx.Exec(
						"INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, ?, ?)",
						repoID, chunkType, chunk,
					)
					if err != nil {
						_ = tx.Rollback()
						errorCount++
						eventCh <- IngestEvent{Type: "error", Repo: fullName, Reason: err.Error()}
						fileTreeSuccess = false
						break
					}
				}
				if !fileTreeSuccess {
					continue
				}
			}

			// Insert sync_state
			_, err = tx.Exec(`
				INSERT INTO sync_state (repo_id, status, last_synced_at)
				VALUES (?, 'completed', ?)
				ON CONFLICT(repo_id) DO UPDATE SET
					status = excluded.status,
					last_synced_at = excluded.last_synced_at,
					error_message = NULL
			`, repoID, syncedAt)
			if err != nil {
				_ = tx.Rollback()
				errorCount++
				eventCh <- IngestEvent{Type: "error", Repo: fullName, Reason: err.Error()}
				continue
			}

			if err := tx.Commit(); err != nil {
				errorCount++
				eventCh <- IngestEvent{Type: "error", Repo: fullName, Reason: err.Error()}
				continue
			}

			totalProcessed++
			status := "new"
			if isExisting {
				status = "updated"
			}
			eventCh <- IngestEvent{Type: "repo", Repo: fullName, Status: status}
		}

		eventCh <- IngestEvent{
			Type:    "done",
			Total:   totalProcessed,
			Skipped: skippedCount,
			Errors:  errorCount,
		}
	}()

	return eventCh
}
