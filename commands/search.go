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
	"github.com/hackastak/repog/internal/format"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/gemini"
	"github.com/hackastak/repog/internal/search"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search across your repositories",
	Long:  "Search for repositories using natural language queries.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSearch,
}

var (
	searchLimit    int
	searchLanguage string
	searchStarred  bool
	searchOwned    bool
	searchOwner    string
)

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "l", 3, "Maximum results to return")
	searchCmd.Flags().StringVar(&searchLanguage, "language", "", "Filter by primary language")
	searchCmd.Flags().BoolVar(&searchStarred, "starred", false, "Only search starred repositories")
	searchCmd.Flags().BoolVar(&searchOwned, "owned", false, "Only search owned repositories")
	searchCmd.Flags().StringVar(&searchOwner, "owner", "", "Filter by repo owner username")
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
	magenta := color.New(color.FgMagenta).SprintFunc()
	blue := color.New(color.FgBlue).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Println(red("Run `repog init` first."))
		os.Exit(1)
	}

	// Create embedding provider
	apiKey, err := config.GetAPIKeyForProvider(cfg.Embedding.Provider)
	if err != nil {
		fmt.Println(red("Failed to get API key:"), err)
		os.Exit(1)
	}

	embedProvider, err := provider.NewEmbeddingProvider(cfg.Embedding, apiKey)
	if err != nil {
		fmt.Println(red("Failed to create embedding provider:"), err)
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
	filters := search.SearchFilters{
		Limit: searchLimit,
	}
	if searchLanguage != "" {
		filters.Language = &searchLanguage
	}
	if searchStarred {
		filters.Starred = &searchStarred
	}
	if searchOwned {
		filters.Owned = &searchOwned
	}
	if searchOwner != "" {
		filters.Owner = &searchOwner
	}

	// Run search with spinner
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Searching..."
	s.Start()

	result, err := search.SearchRepos(commandContext(cmd), database, embedProvider, query, filters)
	s.Stop()

	if err != nil {
		fmt.Println(red("Search failed:"), err)
		os.Exit(1)
	}

	if len(result.Results) == 0 {
		fmt.Println(yellow("\nNo matching repositories found."))
		return nil
	}

	// Render results
	fmt.Println()
	resultWord := "repository"
	if len(result.Results) != 1 {
		resultWord = "repositories"
	}
	fmt.Printf("Found %d matching %s:\n", len(result.Results), resultWord)

	for i, r := range result.Results {
		fmt.Println()

		// Header line
		similarity := green(format.FormatSimilarity(r.Similarity))
		stars := yellow("★ " + format.FormatStars(r.Stars))

		langBadge := ""
		if r.Language != "" {
			langBadge = magenta("[" + r.Language + "]")
		}

		var badges []string
		if r.IsOwned {
			badges = append(badges, blue("owned"))
		}
		if r.IsStarred {
			badges = append(badges, yellow("starred"))
		}
		badgeStr := ""
		if len(badges) > 0 {
			badgeStr = dim("(" + badges[0])
			for i := 1; i < len(badges); i++ {
				badgeStr += ", " + badges[i]
			}
			badgeStr += ")"
		}

		fmt.Printf("%d. %s %s %s %s %s\n", i+1, cyan(r.RepoFullName), similarity, stars, langBadge, badgeStr)

		// Description
		if r.Description != "" {
			fmt.Println("  ", dim(r.Description))
		}

		// URL
		fmt.Println("  ", dim(r.HTMLURL))

		// Matched chunk
		chunkLabel := dim("[" + r.ChunkType + "]")
		content := format.TruncateText(r.Content, 200)
		fmt.Println("  ", chunkLabel, content)
	}

	// Timing metrics
	fmt.Println()
	fmt.Println(dim(fmt.Sprintf("Query: %q | Chunks searched: %d | Embedding: %dms | Search: %dms",
		query, result.TotalConsidered, result.QueryEmbeddingMs, result.SearchMs)))

	return nil
}
