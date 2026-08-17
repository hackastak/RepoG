package ask

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/provider"
	"github.com/hackastak/repog/internal/search"
)

func TestBuildSourceAttributions(t *testing.T) {
	results := []search.SearchResult{
		{RepoFullName: "user/repo1", ChunkType: "readme", Similarity: 0.9},
		{RepoFullName: "user/repo1", ChunkType: "metadata", Similarity: 0.8},
		{RepoFullName: "user/repo2", ChunkType: "readme", Similarity: 0.7},
	}

	sources := buildSourceAttributions(results)

	// Should deduplicate by repo, keeping highest similarity
	if len(sources) != 2 {
		t.Errorf("Expected 2 sources, got %d", len(sources))
	}

	// Check that we keep the highest similarity for deduped repos
	for _, s := range sources {
		if s.RepoFullName == "user/repo1" && s.Similarity != 0.9 {
			t.Errorf("Expected highest similarity 0.9 for user/repo1, got %f", s.Similarity)
		}
	}
}

func TestBuildAskPrompt(t *testing.T) {
	chunks := []search.SearchResult{
		{RepoFullName: "user/repo1", ChunkType: "readme", Content: "This is the README content"},
	}

	prompt := buildAskPrompt("What is this repo about?", chunks)

	if !containsString(prompt, "What is this repo about?") {
		t.Error("Prompt should contain the question")
	}

	if !containsString(prompt, "user/repo1") {
		t.Error("Prompt should contain the repo name")
	}

	if !containsString(prompt, "This is the README content") {
		t.Error("Prompt should contain the chunk content")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func insertTestRepoWithEmbedding(t *testing.T, database *sql.DB, fullName string, content string, embedding []float32) int64 {
	t.Helper()

	result, err := database.Exec(`
		INSERT INTO repos (github_id, full_name, owner, name, description, stars, language, topics, html_url, synced_at)
		VALUES (?, ?, 'owner', 'name', 'desc', 100, 'Go', '[]', 'https://github.com/test', datetime('now'))
	`, 12345, fullName)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	repoID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	chunkResult, err := database.Exec(`
		INSERT INTO chunks (repo_id, chunk_type, content)
		VALUES (?, 'readme', ?)
	`, repoID, content)
	if err != nil {
		t.Fatalf("Failed to insert chunk: %v", err)
	}

	chunkID, err := chunkResult.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get chunk ID: %v", err)
	}

	embeddingBlob := provider.Float32SliceToBytes(embedding)
	_, err = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)", chunkID, embeddingBlob)
	if err != nil {
		t.Fatalf("Failed to insert embedding: %v", err)
	}

	return repoID
}

func TestAskQuestion_ReturnsPopulatedResult(t *testing.T) {
	// Create mock providers
	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		embedding := make([]float32, 768)
		for i := range embedding {
			embedding[i] = 0.5
		}
		return embedding
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		StreamFunc: func(_ context.Context, _ provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
			text := "This is the answer based on the context."
			if onChunk != nil {
				onChunk(text)
			}
			return &provider.LLMResult{Text: text, InputTokens: 100, OutputTokens: 50}, nil
		},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Create embedding that will match query embedding (0.5 values)
	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = 0.5
	}
	insertTestRepoWithEmbedding(t, database, "user/testrepo", "This is repo content.", embedding)

	var receivedChunks []string
	onChunk := func(chunk string) {
		receivedChunks = append(receivedChunks, chunk)
	}

	result, err := AskQuestion(context.Background(), AskOptions{
		Question:          "What is this repo about?",
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
		Limit:             10,
	}, onChunk)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Question != "What is this repo about?" {
		t.Errorf("Expected question preserved, got '%s'", result.Question)
	}

	if result.Answer == "" {
		t.Error("Expected non-empty answer")
	}

	if len(receivedChunks) == 0 {
		t.Error("Expected onChunk to be called")
	}

	if result.DurationMs < 0 {
		t.Errorf("Expected non-negative duration, got %d", result.DurationMs)
	}

	if len(result.Sources) == 0 {
		t.Error("Expected sources to be populated")
	}
}

func TestAskQuestion_RepoFilterScopesResults(t *testing.T) {
	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		embedding := make([]float32, 768)
		for i := range embedding {
			embedding[i] = 0.5
		}
		return embedding
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		StreamFunc: func(_ context.Context, _ provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
			text := "Filtered answer"
			if onChunk != nil {
				onChunk(text)
			}
			return &provider.LLMResult{Text: text, InputTokens: 10, OutputTokens: 5}, nil
		},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Create embeddings
	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = 0.5
	}

	// Insert two repos with unique github_id
	result1, _ := database.Exec(`
		INSERT INTO repos (github_id, full_name, owner, name, description, stars, language, topics, html_url, synced_at)
		VALUES (1001, 'user/repo1', 'user', 'repo1', 'desc', 100, 'Go', '[]', 'https://github.com/user/repo1', datetime('now'))
	`)
	repo1ID, _ := result1.LastInsertId()
	chunk1Result, _ := database.Exec(`INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'readme', 'Repo1 content')`, repo1ID)
	chunk1ID, _ := chunk1Result.LastInsertId()
	_, _ = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)", chunk1ID, provider.Float32SliceToBytes(embedding))

	result2, _ := database.Exec(`
		INSERT INTO repos (github_id, full_name, owner, name, description, stars, language, topics, html_url, synced_at)
		VALUES (1002, 'user/repo2', 'user', 'repo2', 'desc', 100, 'Go', '[]', 'https://github.com/user/repo2', datetime('now'))
	`)
	repo2ID, _ := result2.LastInsertId()
	chunk2Result, _ := database.Exec(`INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'readme', 'Repo2 content')`, repo2ID)
	chunk2ID, _ := chunk2Result.LastInsertId()
	_, _ = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)", chunk2ID, provider.Float32SliceToBytes(embedding))

	// Filter to only user/repo1
	askResult, err := AskQuestion(context.Background(), AskOptions{
		Question:          "What is this repo about?",
		Repo:              "user/repo1",
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
		Limit:             10,
	}, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// All sources should be from user/repo1
	for _, source := range askResult.Sources {
		if source.RepoFullName != "user/repo1" {
			t.Errorf("Expected source from user/repo1, got %s", source.RepoFullName)
		}
	}
}

func TestAskQuestion_ReturnsNoInfoMessageWhenNoResults(t *testing.T) {
	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		embedding := make([]float32, 768)
		return embedding
	}

	// LLM should not be called when no results
	mockLLM := provider.NewMockLLMProvider()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Empty database - no repos

	var receivedChunk string
	onChunk := func(chunk string) {
		receivedChunk = chunk
	}

	result, err := AskQuestion(context.Background(), AskOptions{
		Question:          "What is this repo about?",
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	}, onChunk)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedMsg := "I couldn't find any relevant information in your knowledge base to answer this question."
	if result.Answer != expectedMsg {
		t.Errorf("Expected no info message, got '%s'", result.Answer)
	}

	if receivedChunk != expectedMsg {
		t.Errorf("Expected onChunk to receive no info message, got '%s'", receivedChunk)
	}
}

func TestAskQuestion_HandlesLLMError(t *testing.T) {
	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		embedding := make([]float32, 768)
		for i := range embedding {
			embedding[i] = 0.5
		}
		return embedding
	}

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

	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = 0.5
	}
	insertTestRepoWithEmbedding(t, database, "user/repo", "Content", embedding)

	result, err := AskQuestion(context.Background(), AskOptions{
		Question:          "What is this repo about?",
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	}, nil)

	// An LLM failure must surface as a Go error, not as answer text: callers
	// decide how to report it, and `repog ask` exits non-zero because of it.
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

	// The answer is not filled with an error string masquerading as content.
	if containsString(result.Answer, "Error generating answer") {
		t.Errorf("Error text leaked into Answer: %q", result.Answer)
	}

	// Retrieval succeeded, so the sources it found are still reported.
	if len(result.Sources) == 0 {
		t.Error("Expected sources to be populated even when generation fails")
	}
}

func TestAskQuestion_DefaultLimit(t *testing.T) {
	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		embedding := make([]float32, 768)
		return embedding
	}

	mockLLM := provider.NewMockLLMProvider()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Test with Limit = 0 (should default to 10)
	result, err := AskQuestion(context.Background(), AskOptions{
		Question:          "What is this repo about?",
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
		Limit:             0, // Should default to 10
	}, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Since there's no data, we just verify no panic occurred
	if result.Question != "What is this repo about?" {
		t.Errorf("Expected question preserved, got '%s'", result.Question)
	}
}

func TestAskQuestion_NilOnChunkCallback(t *testing.T) {
	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		embedding := make([]float32, 768)
		for i := range embedding {
			embedding[i] = 0.5
		}
		return embedding
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		StreamFunc: func(_ context.Context, _ provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
			if onChunk != nil {
				onChunk("Answer")
			}
			return &provider.LLMResult{Text: "Answer", InputTokens: 10, OutputTokens: 5}, nil
		},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = 0.5
	}
	insertTestRepoWithEmbedding(t, database, "user/repo", "Content", embedding)

	// Should not panic with nil callback
	result, err := AskQuestion(context.Background(), AskOptions{
		Question:          "What is this repo about?",
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	}, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Answer != "Answer" {
		t.Errorf("Expected 'Answer', got '%s'", result.Answer)
	}
}

func TestAskQuestion_CaseInsensitiveRepoFilter(t *testing.T) {
	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		embedding := make([]float32, 768)
		for i := range embedding {
			embedding[i] = 0.5
		}
		return embedding
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		StreamFunc: func(_ context.Context, _ provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
			text := "Filtered answer"
			if onChunk != nil {
				onChunk(text)
			}
			return &provider.LLMResult{Text: text, InputTokens: 10, OutputTokens: 5}, nil
		},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = 0.5
	}
	insertTestRepoWithEmbedding(t, database, "User/MyRepo", "Content", embedding)

	// Filter with different case - should still match
	result, err := AskQuestion(context.Background(), AskOptions{
		Question:          "What is this repo about?",
		Repo:              "user/myrepo", // lowercase
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	}, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should have found the repo (case-insensitive match)
	if len(result.Sources) == 0 {
		t.Error("Expected to find sources with case-insensitive match")
	}
}

func TestBuildSourceAttributions_CapsAtFive(t *testing.T) {
	results := []search.SearchResult{
		{RepoFullName: "user/repo1", ChunkType: "readme", Similarity: 0.9},
		{RepoFullName: "user/repo2", ChunkType: "readme", Similarity: 0.8},
		{RepoFullName: "user/repo3", ChunkType: "readme", Similarity: 0.7},
		{RepoFullName: "user/repo4", ChunkType: "readme", Similarity: 0.6},
		{RepoFullName: "user/repo5", ChunkType: "readme", Similarity: 0.5},
		{RepoFullName: "user/repo6", ChunkType: "readme", Similarity: 0.4},
		{RepoFullName: "user/repo7", ChunkType: "readme", Similarity: 0.3},
	}

	sources := buildSourceAttributions(results)

	if len(sources) > 5 {
		t.Errorf("Expected at most 5 sources, got %d", len(sources))
	}
}
