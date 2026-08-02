package summarize

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/provider"
)

func TestBuildSummarizePrompt(t *testing.T) {
	chunks := []chunkRecord{
		{ChunkType: "metadata", Content: `{"full_name": "user/repo"}`},
		{ChunkType: "readme", Content: "# My Repo\nThis is a test repo."},
	}

	prompt := buildSummarizePrompt("user/repo", chunks)

	if !containsString(prompt, "user/repo") {
		t.Error("Prompt should contain repo name")
	}

	if !containsString(prompt, "metadata") {
		t.Error("Prompt should contain chunk type")
	}

	if !containsString(prompt, "# My Repo") {
		t.Error("Prompt should contain chunk content")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func insertTestRepoWithChunks(t *testing.T, database *sql.DB, fullName string, chunks []chunkRecord) int64 {
	t.Helper()

	result, err := database.Exec(`
		INSERT INTO repos (github_id, full_name, owner, name, description, stars, language, topics, synced_at)
		VALUES (?, ?, 'owner', 'name', 'desc', 100, 'Go', '[]', datetime('now'))
	`, 12345, fullName)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	repoID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	for _, chunk := range chunks {
		_, err := database.Exec(`
			INSERT INTO chunks (repo_id, chunk_type, content)
			VALUES (?, ?, ?)
		`, repoID, chunk.ChunkType, chunk.Content)
		if err != nil {
			t.Fatalf("Failed to insert chunk: %v", err)
		}
	}

	return repoID
}

func TestSummarizeRepo_ReturnsPopulatedResult(t *testing.T) {
	// Create mock LLM provider
	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		StreamFunc: func(_ context.Context, _ provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
			chunks := []string{
				"## Overview\nThis is a test repo.",
				"\n\n## Tech Stack\nGo language.",
				"\n\n## Use Cases\nTesting.",
			}
			fullText := ""
			for _, chunk := range chunks {
				fullText += chunk
				if onChunk != nil {
					onChunk(chunk)
				}
			}
			return &provider.LLMResult{Text: fullText, InputTokens: 100, OutputTokens: 50}, nil
		},
	}

	// Set up test database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Insert test data
	insertTestRepoWithChunks(t, database, "user/testrepo", []chunkRecord{
		{ChunkType: "metadata", Content: `{"full_name": "user/testrepo"}`},
		{ChunkType: "readme", Content: "# Test Repo\nA test repository."},
	})

	// Track chunks received
	var receivedChunks []string
	onChunk := func(chunk string) {
		receivedChunks = append(receivedChunks, chunk)
	}

	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:        "user/testrepo",
		DB:          database,
		LLMProvider: mockLLM,
	}, onChunk)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Repo != "user/testrepo" {
		t.Errorf("Expected repo 'user/testrepo', got '%s'", result.Repo)
	}

	if result.ChunksUsed != 2 {
		t.Errorf("Expected 2 chunks used, got %d", result.ChunksUsed)
	}

	if !containsString(result.Summary, "## Overview") {
		t.Errorf("Expected summary to contain '## Overview', got '%s'", result.Summary)
	}

	if len(receivedChunks) == 0 {
		t.Error("Expected onChunk to be called")
	}

	if result.DurationMs < 0 {
		t.Errorf("Expected non-negative duration, got %d", result.DurationMs)
	}
}

func TestSummarizeRepo_CaseInsensitiveMatch(t *testing.T) {
	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		StreamFunc: func(_ context.Context, _ provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
			if onChunk != nil {
				onChunk("Summary")
			}
			return &provider.LLMResult{Text: "Summary", InputTokens: 10, OutputTokens: 5}, nil
		},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Insert with lowercase
	insertTestRepoWithChunks(t, database, "user/myrepo", []chunkRecord{
		{ChunkType: "readme", Content: "# My Repo"},
	})

	// Query with different case
	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:        "USER/MYREPO",
		DB:          database,
		LLMProvider: mockLLM,
	}, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.ChunksUsed != 1 {
		t.Errorf("Expected 1 chunk used (case-insensitive match), got %d", result.ChunksUsed)
	}
}

func TestSummarizeRepo_ReturnsNoDataMessageWhenNoChunks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	var receivedChunk string
	onChunk := func(chunk string) {
		receivedChunk = chunk
	}

	// LLM provider should not be called when no chunks exist
	mockLLM := provider.NewMockLLMProvider()

	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:        "nonexistent/repo",
		DB:          database,
		LLMProvider: mockLLM,
	}, onChunk)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedMsg := "No data found for this repository. Try running `repog sync` and `repog embed` first."
	if result.Summary != expectedMsg {
		t.Errorf("Expected no data message, got '%s'", result.Summary)
	}

	if receivedChunk != expectedMsg {
		t.Errorf("Expected onChunk to receive no data message, got '%s'", receivedChunk)
	}

	if result.ChunksUsed != 0 {
		t.Errorf("Expected 0 chunks used, got %d", result.ChunksUsed)
	}
}

func TestSummarizeRepo_HandlesLLMError(t *testing.T) {
	// Create mock LLM provider that returns an error
	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		StreamFunc: func(_ context.Context, _ provider.LLMRequest, _ func(string)) (*provider.LLMResult, *provider.LLMError) {
			return nil, &provider.LLMError{Message: "Internal Server Error", StatusCode: 500}
		},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	insertTestRepoWithChunks(t, database, "user/repo", []chunkRecord{
		{ChunkType: "readme", Content: "# Repo"},
	})

	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:        "user/repo",
		DB:          database,
		LLMProvider: mockLLM,
	}, nil)

	// An LLM failure must surface as a Go error, not as summary text: callers
	// decide how to report it, and `repog summarize` exits non-zero because of it.
	if err == nil {
		t.Fatal("Expected an error when the LLM call fails, got nil")
	}

	// The underlying *provider.LLMError must survive wrapping, so callers can
	// inspect the status code (e.g. to tell a retryable 429 from a fatal 401).
	var llmErr *provider.LLMError
	if !errors.As(err, &llmErr) {
		t.Fatalf("Expected error to wrap *provider.LLMError, got %T: %v", err, err)
	}
	if llmErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", llmErr.StatusCode)
	}

	// The summary is not filled with an error string masquerading as content.
	if containsString(result.Summary, "Error generating summary") {
		t.Errorf("Error text leaked into Summary: %q", result.Summary)
	}
}

func TestSummarizeRepo_NilOnChunkCallback(t *testing.T) {
	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		StreamFunc: func(_ context.Context, _ provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
			if onChunk != nil {
				onChunk("Summary")
			}
			return &provider.LLMResult{Text: "Summary", InputTokens: 10, OutputTokens: 5}, nil
		},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	insertTestRepoWithChunks(t, database, "user/repo", []chunkRecord{
		{ChunkType: "readme", Content: "# Repo"},
	})

	// Should not panic with nil callback
	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:        "user/repo",
		DB:          database,
		LLMProvider: mockLLM,
	}, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Summary != "Summary" {
		t.Errorf("Expected 'Summary', got '%s'", result.Summary)
	}
}
