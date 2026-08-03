package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/gemini"
	"github.com/hackastak/repog/internal/summarize"
)

var summarizeCmd = &cobra.Command{
	Use:   "summarize <owner/repo>",
	Short: "Generate an AI summary of a repository",
	Long:  "Generate a structured summary of a repository from your knowledge base.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSummarize,
}

func runSummarize(cmd *cobra.Command, args []string) error {
	repo := args[0]

	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

	// Validate repo format
	if !strings.Contains(repo, "/") {
		fmt.Println(red("Invalid repository format. Use owner/repo (e.g., hackastak/repog)"))
		os.Exit(1)
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Println(red("Run `repog init` first."))
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

	// Check if repo exists in database
	var repoCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM repos WHERE LOWER(full_name) = LOWER(?)", repo).Scan(&repoCount); err != nil {
		fmt.Println(red("Database error:"), err)
		os.Exit(1)
	}
	if repoCount == 0 {
		fmt.Println(yellow("Repository not found in knowledge base:"), repo)
		fmt.Println(dim("Run `repog sync` to add repositories."))
		os.Exit(1)
	}

	// Check if repo has chunks
	var chunkCount int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM chunks c
		JOIN repos r ON r.id = c.repo_id
		WHERE LOWER(r.full_name) = LOWER(?)
	`, repo).Scan(&chunkCount); err != nil {
		fmt.Println(red("Database error:"), err)
		os.Exit(1)
	}
	if chunkCount == 0 {
		fmt.Println(yellow("No data found for repository:"), repo)
		fmt.Println(dim("Run `repog sync` to fetch repository data."))
		os.Exit(1)
	}

	// Print header
	fmt.Println()
	fmt.Println(bold("Summary:"), cyan(repo))
	fmt.Println()

	// Stream summary
	result, err := summarize.SummarizeRepo(commandContext(cmd), summarize.SummarizeOptions{
		Repo:        repo,
		DB:          database,
		LLMProvider: llmProvider,
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

	// Print metrics
	fmt.Println(dim(fmt.Sprintf("Chunks used: %d | Tokens: %d in / %d out | Duration: %dms",
		result.ChunksUsed, result.InputTokens, result.OutputTokens, result.DurationMs)))

	return nil
}
