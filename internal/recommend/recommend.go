// Package recommend provides AI-powered repository recommendations.
package recommend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hackastak/repog/internal/provider"
	"github.com/hackastak/repog/internal/search"
)

// RecommendOptions configures the recommendation query.
type RecommendOptions struct {
	Query             string
	Limit             int
	Filters           search.SearchFilters
	DB                *sql.DB
	EmbeddingProvider provider.EmbeddingProvider
	LLMProvider       provider.LLMProvider
}

// Recommendation represents a single repository recommendation.
type Recommendation struct {
	Rank         int
	RepoFullName string
	HTMLURL      string
	Reasoning    string
}

// RecommendResult contains the recommendation results.
type RecommendResult struct {
	Recommendations      []Recommendation
	Query                string
	CandidatesConsidered int
	InputTokens          int
	OutputTokens         int
	DurationMs           int64
}

const systemPrompt = `You are a developer tool assistant. Your job is to recommend the most relevant GitHub repositories from a provided list based on the user's query. You must respond with valid JSON only — no markdown, no explanation outside the JSON.`

// buildRecommendPrompt builds the user prompt for recommendations.
func buildRecommendPrompt(query string, candidates []search.SearchResult, limit int) string {
	candidateLines := make([]string, 0, len(candidates))
	for _, c := range candidates {
		description := c.Description
		if description == "" {
			description = "No description"
		}
		language := c.Language
		if language == "" {
			language = "Unknown language"
		}
		candidateLines = append(candidateLines, fmt.Sprintf("- %s: %s (%s) — %s", c.RepoFullName, description, language, c.HTMLURL))
	}

	return fmt.Sprintf(`Query: %s

Here are the candidate repositories:
%s

Based on the query, recommend the top %d most relevant repositories.

Respond with a JSON array of objects in this exact format:
[
  {
    "rank": 1,
    "repoFullName": "owner/repo",
    "htmlUrl": "https://github.com/owner/repo",
    "reasoning": "One or two sentences explaining why this repo is relevant to the query."
  }
]

Only include repositories from the provided list. Do not invent repositories.
Rank by relevance to the query. Be concise in reasoning.`, query, strings.Join(candidateLines, "\n"), limit)
}

// stripCodeFences removes markdown code fences from the response.
func stripCodeFences(text string) string {
	cleaned := strings.TrimSpace(text)

	// Strategy 1: Look for content inside ```json ... ``` or ``` ... ``` blocks
	codeBlockRegex := regexp.MustCompile("(?i)```(?:json)?\\s*([\\s\\S]*?)```")
	if matches := codeBlockRegex.FindStringSubmatch(cleaned); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Strategy 2: Look for the first '[' and last ']' if we expect an array
	startArray := strings.Index(cleaned, "[")
	endArray := strings.LastIndex(cleaned, "]")
	if startArray != -1 && endArray != -1 && endArray > startArray {
		return strings.TrimSpace(cleaned[startArray : endArray+1])
	}

	// Strategy 3: Look for the first '{' and last '}'
	startObj := strings.Index(cleaned, "{")
	endObj := strings.LastIndex(cleaned, "}")
	if startObj != -1 && endObj != -1 && endObj > startObj {
		return strings.TrimSpace(cleaned[startObj : endObj+1])
	}

	return cleaned
}

// parseRecommendations parses the LLM response into recommendations.
func parseRecommendations(text string, limit int) []Recommendation {
	cleaned := stripCodeFences(text)

	var parsed []interface{}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		// Try parsing as single object
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(cleaned), &obj); err != nil {
			return nil
		}
		parsed = []interface{}{obj}
	}

	var recommendations []Recommendation
	for _, item := range parsed {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		repoFullName, _ := obj["repoFullName"].(string)
		htmlURL, _ := obj["htmlUrl"].(string)
		reasoning, _ := obj["reasoning"].(string)
		rank, _ := obj["rank"].(float64)

		if repoFullName == "" || htmlURL == "" || reasoning == "" {
			continue
		}

		r := Recommendation{
			Rank:         int(rank),
			RepoFullName: repoFullName,
			HTMLURL:      htmlURL,
			Reasoning:    reasoning,
		}
		if r.Rank == 0 {
			r.Rank = len(recommendations) + 1
		}

		recommendations = append(recommendations, r)

		if len(recommendations) >= limit {
			break
		}
	}

	return recommendations
}

// RecommendRepos gets AI-powered repository recommendations.
func RecommendRepos(ctx context.Context, opts RecommendOptions) (RecommendResult, error) {
	start := time.Now()
	result := RecommendResult{
		Query:           opts.Query,
		Recommendations: make([]Recommendation, 0),
	}

	// Set default limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 3
	}

	// Search for candidates (over-fetch for LLM context)
	candidateLimit := limit * 5
	opts.Filters.Limit = candidateLimit

	searchResult, err := search.SearchRepos(ctx, opts.DB, opts.EmbeddingProvider, opts.Query, opts.Filters)
	if err != nil {
		result.DurationMs = time.Since(start).Milliseconds()
		return result, err
	}

	if len(searchResult.Results) == 0 {
		result.DurationMs = time.Since(start).Milliseconds()
		return result, nil
	}

	result.CandidatesConsidered = len(searchResult.Results)

	// Build prompt
	prompt := buildRecommendPrompt(opts.Query, searchResult.Results, limit)

	// Call LLM (using non-streaming to avoid truncation issues)
	llmResult, llmErr := opts.LLMProvider.Call(ctx, provider.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
		MaxTokens:    2048,
	})

	result.DurationMs = time.Since(start).Milliseconds()

	if llmErr != nil {
		return result, fmt.Errorf("generating recommendations: %w", llmErr)
	}

	result.InputTokens = llmResult.InputTokens
	result.OutputTokens = llmResult.OutputTokens

	// Parse recommendations
	result.Recommendations = parseRecommendations(llmResult.Text, limit)

	return result, nil
}
