package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/gemini"
	"github.com/hackastak/repog/internal/recommend"
	"github.com/hackastak/repog/internal/search"
)

var recommendCmd = &cobra.Command{
	Use:   "recommend <query>",
	Short: "Get AI-powered repository recommendations",
	Long:  "Get intelligent repository recommendations based on your query.",
	Args:  cobra.ExactArgs(1),
	RunE:  runRecommend,
}

var (
	recommendLimit    int
	recommendLanguage string
	recommendStarred  bool
	recommendOwned    bool
	recommendOwner    string
)

func init() {
	recommendCmd.Flags().IntVarP(&recommendLimit, "limit", "l", 3, "Maximum recommendations to return")
	recommendCmd.Flags().StringVar(&recommendLanguage, "language", "", "Filter by primary language")
	recommendCmd.Flags().BoolVar(&recommendStarred, "starred", false, "Only recommend from starred repositories")
	recommendCmd.Flags().BoolVar(&recommendOwned, "owned", false, "Only recommend from owned repositories")
	recommendCmd.Flags().StringVar(&recommendOwner, "owner", "", "Filter by repo owner username")
}

func runRecommend(cmd *cobra.Command, args []string) error {
	query := args[0]

	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

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

	// Build filters
	filters := search.SearchFilters{}
	if recommendLanguage != "" {
		filters.Language = &recommendLanguage
	}
	if recommendStarred {
		filters.Starred = &recommendStarred
	}
	if recommendOwned {
		filters.Owned = &recommendOwned
	}
	if recommendOwner != "" {
		filters.Owner = &recommendOwner
	}

	// Run recommendation with spinner
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Getting recommendations..."
	s.Start()

	result, err := recommend.RecommendRepos(commandContext(cmd), recommend.RecommendOptions{
		Query:             query,
		Limit:             recommendLimit,
		Filters:           filters,
		DB:                database,
		EmbeddingProvider: embedProvider,
		LLMProvider:       llmProvider,
	})
	s.Stop()

	if err != nil {
		fmt.Println(red("Recommendation failed:"), err)
		os.Exit(1)
	}

	if len(result.Recommendations) == 0 {
		fmt.Println(yellow("\nNo recommendations found."))
		fmt.Println(dim(fmt.Sprintf("Query: %q | Candidates: %d | Duration: %dms",
			query, result.CandidatesConsidered, result.DurationMs)))
		return nil
	}

	// Render recommendations
	fmt.Println()
	fmt.Println(bold("Recommendations:"))
	fmt.Println()

	for _, r := range result.Recommendations {
		fmt.Printf("%s %d. %s\n", green("▸"), r.Rank, cyan(r.RepoFullName))
		fmt.Println("  ", dim(r.HTMLURL))
		fmt.Println("  ", r.Reasoning)
		fmt.Println()
	}

	// Timing metrics
	fmt.Println(dim(fmt.Sprintf("Query: %q | Candidates: %d | Duration: %dms",
		query, result.CandidatesConsidered, result.DurationMs)))

	return nil
}
