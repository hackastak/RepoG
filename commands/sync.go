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
	_ "github.com/hackastak/repog/internal/provider/ollama"
	_ "github.com/hackastak/repog/internal/provider/openai"
	_ "github.com/hackastak/repog/internal/provider/openrouter"
	_ "github.com/hackastak/repog/internal/provider/voyageai"
	"github.com/hackastak/repog/internal/sync"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync repositories from GitHub to local database",
	Long:  "Fetch repository metadata, READMEs, and file trees from GitHub and store them locally.",
	RunE:  runSync,
}

var (
	syncOwned    bool
	syncStarred  bool
	syncFullTree bool
	syncVerbose  bool
)

func init() {
	syncCmd.Flags().BoolVar(&syncOwned, "owned", false, "Sync owned repositories")
	syncCmd.Flags().BoolVar(&syncStarred, "starred", false, "Sync starred repositories")
	syncCmd.Flags().BoolVar(&syncFullTree, "full-tree", false, "Always fetch file trees regardless of README length")
	syncCmd.Flags().BoolVar(&syncVerbose, "verbose", false, "Show detailed progress per repository")
}

func runSync(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	// Default to both if no flags specified
	if !syncOwned && !syncStarred {
		syncOwned = true
		syncStarred = true
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Run `repog init` first."))
		os.Exit(1)
	}

	githubPAT, err := config.GetGitHubPAT()
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Run `repog init` first."))
		os.Exit(1)
	}

	// Open database
	database, err := db.Open(cfg.DBPath, cfg.Embedding.Dimensions)
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Database error:"), err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	// Calculate chunk size based on embedding provider's token limit
	embeddingAPIKey, err := config.GetAPIKeyForProvider(cfg.Embedding.Provider)
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Failed to get embedding API key:"), err)
		os.Exit(1)
	}

	embedProvider, err := provider.NewEmbeddingProvider(cfg.Embedding, embeddingAPIKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Failed to create embedding provider:"), err)
		os.Exit(1)
	}

	maxChunkSize := sync.CalculateMaxCharsFromTokens(embedProvider.MaxTokens())

	if syncVerbose {
		fmt.Printf("Using chunk size: %d characters (based on %d token limit)\n", maxChunkSize, embedProvider.MaxTokens())
	}

	// Start ingestion
	var s *spinner.Spinner
	if !syncVerbose {
		s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Syncing repositories..."
		s.Start()
	}

	var syncedCount, skippedCount, errorCount int

	eventCh := sync.IngestRepos(commandContext(cmd), sync.IngestOptions{
		IncludeOwned:   syncOwned,
		IncludeStarred: syncStarred,
		FullTree:       syncFullTree,
		MaxChunkSize:   maxChunkSize,
		DB:             database,
		GitHubPAT:      githubPAT,
	})

	for event := range eventCh {
		switch event.Type {
		case "repo":
			syncedCount++
			if syncVerbose {
				statusText := "new    "
				if event.Status == "updated" {
					statusText = "updated"
				}
				fmt.Println(green("✓"), statusText, event.Repo)
			} else if s != nil {
				s.Suffix = fmt.Sprintf(" Syncing repositories... (%d synced, %d skipped)", syncedCount, skippedCount)
			}

		case "skip":
			skippedCount++
			if syncVerbose {
				fmt.Println(yellow("~"), "skipped ", event.Repo, "("+event.Reason+")")
			} else if s != nil {
				s.Suffix = fmt.Sprintf(" Syncing repositories... (%d synced, %d skipped)", syncedCount, skippedCount)
			}

		case "error":
			errorCount++
			if syncVerbose {
				fmt.Println(red("✗"), "error   ", event.Repo, "("+event.Reason+")")
			}

		case "done":
			if s != nil {
				s.Stop()
			}

			if event.Errors > 0 {
				fmt.Println(yellow("✓"), fmt.Sprintf("Sync complete — %d repos synced, %d skipped, %d errors",
					event.Total, event.Skipped, event.Errors))
			} else {
				fmt.Println(green("✓"), fmt.Sprintf("Sync complete — %d repos synced, %d skipped, %d errors",
					event.Total, event.Skipped, event.Errors))
			}
		}
	}

	if errorCount > 0 {
		os.Exit(1)
	}

	return nil
}
