package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/ask"
	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/format"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/gemini"
)

var askCmd = &cobra.Command{
	Use:   "ask <question>",
	Short: "Ask a question about your repositories",
	Long:  "Use RAG to answer questions about your repository knowledge base.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAsk,
}

var (
	askRepo  string
	askLimit int
)

func init() {
	askCmd.Flags().StringVar(&askRepo, "repo", "", "Scope to a specific repository (owner/repo)")
	askCmd.Flags().IntVar(&askLimit, "limit", 10, "Number of chunks to retrieve as context")
}

func runAsk(cmd *cobra.Command, args []string) error {
	question := args[0]

	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

	// Validate repo format if provided
	if askRepo != "" && !strings.Contains(askRepo, "/") {
		fmt.Println(red("Invalid repository format. Use owner/repo (e.g., hackastak/repog)"))
		os.Exit(1)
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Println(red("Run `repog init` first."))
		os.Exit(1)
	}

	// Create embedding provider
	embedKey, err := config.GetAPIKeyForProvider(cfg.Embedding.Provider)
	if err != nil {
		fmt.Println(red("Failed to get embedding API key:"), err)
		os.Exit(1)
	}

	embedProvider, err := provider.NewEmbeddingProvider(cfg.Embedding, embedKey)
	if err != nil {
		fmt.Println(red("Failed to create embedding provider:"), err)
		os.Exit(1)
	}

	// Create LLM provider
	genKey, err := config.GetAPIKeyForProvider(cfg.Generation.Provider)
	if err != nil {
		fmt.Println(red("Failed to get generation API key:"), err)
		os.Exit(1)
	}

	llmProvider, err := provider.NewLLMProvider(cfg.Generation, genKey)
	if err != nil {
		fmt.Println(red("Failed to create LLM provider:"), err)
		os.Exit(1)
	}

	// Open database
	database, err := db.Open(cfg.DBPath, cfg.Embedding.Dimensions)
	if err != nil {
		fmt.Println(red("Database error:"), err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	// Check if embeddings exist
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM chunk_embeddings").Scan(&count); err != nil {
		fmt.Println(red("Database error:"), err)
		os.Exit(1)
	}
	if count == 0 {
		fmt.Println(red("No embeddings found. Run `repog sync` then `repog embed` first."))
		os.Exit(1)
	}

	// Print question header
	fmt.Println()
	fmt.Println(bold("Question:"), question)
	fmt.Println()
	fmt.Print(bold("Answer: "))

	// Stream answer
	result, err := ask.AskQuestion(commandContext(cmd), ask.AskOptions{
		Question:          question,
		Repo:              askRepo,
		Limit:             askLimit,
		DB:                database,
		EmbeddingProvider: embedProvider,
		LLMProvider:       llmProvider,
	}, func(chunk string) {
		fmt.Print(chunk)
	})

	if err != nil {
		fmt.Println()
		fmt.Println(red("Error:"), err)
		os.Exit(1)
	}

	fmt.Println() // End the streaming line
	fmt.Println()

	// Print sources
	if len(result.Sources) > 0 {
		fmt.Println(dim("Sources:"))
		for _, source := range result.Sources {
			similarity := format.FormatSimilarity(source.Similarity)
			fmt.Println(" ", cyan(source.RepoFullName), dim("("+source.ChunkType+")"), dim(similarity))
		}
		fmt.Println()
	}

	// Print metrics
	fmt.Println(dim(fmt.Sprintf("Tokens: %d in / %d out | Duration: %dms",
		result.InputTokens, result.OutputTokens, result.DurationMs)))

	return nil
}
