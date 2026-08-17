// Package summarize provides AI-powered repository summarization.
package summarize

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hackastak/repog/internal/provider"
)

// SummarizeOptions configures the summarization.
type SummarizeOptions struct {
	Repo        string // full_name, required
	DB          *sql.DB
	LLMProvider provider.LLMProvider
}

// SummarizeResult contains the summarization result.
type SummarizeResult struct {
	Summary      string
	Repo         string
	ChunksUsed   int
	InputTokens  int
	OutputTokens int
	DurationMs   int64
}

const systemPrompt = `You are a technical documentation assistant. Your job is to produce a clear,
structured summary of a GitHub repository based on its metadata, README, and
file tree. Always respond in exactly three sections with these exact headings:

## Overview
## Tech Stack
## Use Cases

Be concise. Use plain prose — no bullet points. Each section should be 2-4 sentences.`

// chunkRecord represents a chunk from the database.
type chunkRecord struct {
	ChunkType string
	Content   string
}

// buildSummarizePrompt builds the summarization prompt.
func buildSummarizePrompt(repo string, chunks []chunkRecord) string {
	contextParts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		contextParts = append(contextParts, fmt.Sprintf("--- %s ---\n%s", chunk.ChunkType, chunk.Content))
	}

	contextBlock := strings.Join(contextParts, "\n\n")

	return fmt.Sprintf(`Please summarize the following GitHub repository.

Repository: %s

Context:
%s`, repo, contextBlock)
}

// SummarizeRepo generates a structured AI summary of a repository.
func SummarizeRepo(ctx context.Context, opts SummarizeOptions, onChunk func(string)) (SummarizeResult, error) {
	start := time.Now()
	result := SummarizeResult{
		Repo: opts.Repo,
	}

	// Query chunks for the repo (case-insensitive)
	rows, err := opts.DB.QueryContext(ctx, `
		SELECT c.chunk_type, c.content
		FROM chunks c
		JOIN repos r ON r.id = c.repo_id
		WHERE LOWER(r.full_name) = LOWER(?)
		ORDER BY c.chunk_type ASC
	`, opts.Repo)
	if err != nil {
		result.DurationMs = time.Since(start).Milliseconds()
		return result, fmt.Errorf("querying chunks for %s: %w", opts.Repo, err)
	}
	defer func() { _ = rows.Close() }()

	var chunks []chunkRecord
	for rows.Next() {
		var c chunkRecord
		if err := rows.Scan(&c.ChunkType, &c.Content); err != nil {
			continue
		}
		chunks = append(chunks, c)
	}

	// Handle no chunks
	if len(chunks) == 0 {
		result.Summary = "No data found for this repository. Try running `repog sync` and `repog embed` first."
		if onChunk != nil {
			onChunk(result.Summary)
		}
		result.DurationMs = time.Since(start).Milliseconds()
		return result, nil
	}

	result.ChunksUsed = len(chunks)

	// Build prompt
	prompt := buildSummarizePrompt(opts.Repo, chunks)

	// Stream LLM response
	llmResult, llmErr := opts.LLMProvider.Stream(ctx, provider.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
	}, onChunk)

	result.DurationMs = time.Since(start).Milliseconds()

	if llmErr != nil {
		return result, fmt.Errorf("generating summary for %s: %w", opts.Repo, llmErr)
	}

	result.Summary = llmResult.Text
	result.InputTokens = llmResult.InputTokens
	result.OutputTokens = llmResult.OutputTokens

	return result, nil
}
