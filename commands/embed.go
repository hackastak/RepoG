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
	"github.com/hackastak/repog/internal/embed"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/gemini"
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Generate embeddings for synced repositories",
	Long:  "Generate vector embeddings for repository chunks using Google Gemini.",
	RunE:  runEmbed,
}

var (
	embedIncludeFileTree bool
	embedVerbose         bool
	embedBatchSize       int
)

func init() {
	embedCmd.Flags().BoolVar(&embedIncludeFileTree, "include-file-tree", true, "Include file_tree chunks in embeddings (default true)")
	embedCmd.Flags().BoolVar(&embedVerbose, "verbose", false, "Show detailed progress")
	embedCmd.Flags().IntVar(&embedBatchSize, "batch-size", 20, "Embedding batch size (max 100)")
}

func runEmbed(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Run `repog init` first."))
		os.Exit(1)
	}

	// Create embedding provider
	apiKey, err := config.GetAPIKeyForProvider(cfg.Embedding.Provider)
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Failed to get API key:"), err)
		os.Exit(1)
	}

	embedProvider, err := provider.NewEmbeddingProvider(cfg.Embedding, apiKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Failed to create embedding provider:"), err)
		os.Exit(1)
	}

	// Open database
	database, err := db.Open(cfg.DBPath, cfg.Embedding.Dimensions)
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Database error:"), err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	// Start embedding pipeline
	var s *spinner.Spinner
	if !embedVerbose {
		s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Generating embeddings..."
		s.Start()
	}

	eventCh := embed.RunEmbedPipeline(commandContext(cmd), embed.EmbedOptions{
		IncludeFileTree:   embedIncludeFileTree,
		BatchSize:         embedBatchSize,
		DB:                database,
		EmbeddingProvider: embedProvider,
	})

	for event := range eventCh {
		switch event.Type {
		case "batch":
			if embedVerbose {
				fmt.Printf("Batch %d/%d: %d embedded, %d skipped, %d errors\n",
					event.BatchIndex, event.BatchTotal, event.ChunksEmbedded, event.ChunksSkipped, event.ChunksErrored)
				// Show unique errors from this batch
				if len(event.Errors) > 0 {
					uniqueErrors := make(map[string]bool)
					for _, err := range event.Errors {
						if !uniqueErrors[err] {
							fmt.Println(red("  Error:"), err)
							uniqueErrors[err] = true
						}
					}
				}
			} else if s != nil {
				progress := float64(event.BatchIndex) / float64(event.BatchTotal) * 100
				s.Suffix = fmt.Sprintf(" Generating embeddings... %.0f%% (%d/%d chunks)",
					progress, event.ChunksEmbedded, event.TotalChunks)
			}

		case "repo_skip":
			if embedVerbose {
				fmt.Println(yellow("~"), "skipped", event.RepoFullName)
			}

		case "error":
			// Stop the spinner so a cancellation or failure doesn't leave it
			// spinning forever; the message stays gated on verbose as before.
			if s != nil {
				s.Stop()
			}
			if embedVerbose {
				fmt.Println(red("✗"), "error:", event.RepoFullName)
			}

		case "done":
			if s != nil {
				s.Stop()
			}

			if event.ChunksErrored > 0 {
				fmt.Println(yellow("✓"), fmt.Sprintf("Embedding complete — %d embedded, %d skipped, %d errors",
					event.ChunksEmbedded, event.ChunksSkipped, event.ChunksErrored))
			} else {
				fmt.Println(green("✓"), fmt.Sprintf("Embedding complete — %d embedded, %d skipped, %d errors",
					event.ChunksEmbedded, event.ChunksSkipped, event.ChunksErrored))
			}
		}
	}

	return nil
}
