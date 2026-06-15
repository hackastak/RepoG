package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/format"
	"github.com/hackastak/repog/internal/status"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync and embedding status",
	Long:  "Display the current status of your RepoG knowledge base.",
	RunE:  runStatus,
}

var statusJSON bool

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
}


func runStatus(cmd *cobra.Command, args []string) error {
	red := color.New(color.FgRed).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

	// Load config
	cfg, err := config.LoadConfig()
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

	// Show spinner for plain text mode
	var s *spinner.Spinner
	if !statusJSON {
		s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Fetching status..."
		s.Start()
	}

	ctx := context.Background()

	// Gather local stats (fast, no network).
	result, err := status.Collect(ctx, database, cfg.DBPath)
	if err != nil {
		if s != nil {
			s.Stop()
		}
		fmt.Fprintln(os.Stderr, red("Status error:"), err)
		os.Exit(1)
	}

	// GitHub rate limit is a network call; tolerate failure as "unavailable".
	if pat, patErr := config.GetGitHubPAT(); patErr == nil {
		result.RateLimit = status.FetchRateLimit(ctx, pat)
	}

	if s != nil {
		s.Stop()
	}

	// Output
	if statusJSON {
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))
		return nil
	}

	// Plain text output
	labelWidth := 18
	padLabel := func(label string) string {
		return fmt.Sprintf("%-*s", labelWidth, label)
	}

	fmt.Println(bold("RepoG Status"))
	fmt.Println(dim("─────────────────────────────────────────────"))
	fmt.Println()

	// Repositories
	fmt.Println(bold("  Repositories"))
	fmt.Printf("    %s%15d\n", padLabel("Total:"), result.Repos.Total)
	fmt.Printf("    %s%15d\n", padLabel("Owned:"), result.Repos.Owned)
	fmt.Printf("    %s%15d\n", padLabel("Starred:"), result.Repos.Starred)
	fmt.Printf("    %s%15d\n", padLabel("Embedded:"), result.Repos.EmbeddedCount)
	fmt.Printf("    %s%15d\n", padLabel("Pending embed:"), result.Repos.PendingEmbed)
	fmt.Println()

	// Knowledge Base
	fmt.Println(bold("  Knowledge Base"))
	fmt.Printf("    %s%15d\n", padLabel("Chunks:"), result.Embed.TotalChunks)
	fmt.Printf("    %s%15d\n", padLabel("Embeddings:"), result.Embed.TotalEmbeddings)
	fmt.Println()

	// Last Sync
	fmt.Println(bold("  Last Sync"))
	syncStatus := "Never synced"
	if result.Sync.LastSyncStatus != nil {
		syncStatus = *result.Sync.LastSyncStatus
	}

	statusColor := syncStatus
	switch syncStatus {
	case "completed":
		statusColor = green(syncStatus)
	case "failed":
		statusColor = red(syncStatus)
	case "in_progress":
		statusColor = yellow(syncStatus)
	}
	fmt.Printf("    %s%15s\n", padLabel("Status:"), statusColor)

	if result.Sync.LastSyncedAt != nil {
		fmt.Printf("    %s%15s\n", padLabel("Date:"), format.FormatRelativeTime(*result.Sync.LastSyncedAt))
	}
	fmt.Println()

	// Last Embed
	fmt.Println(bold("  Last Embed"))
	if result.Embed.LastEmbeddedAt != nil {
		fmt.Printf("    %s%15s\n", padLabel("Date:"), format.FormatRelativeTime(*result.Embed.LastEmbeddedAt))
	} else {
		fmt.Printf("    %s%15s\n", padLabel("Date:"), "Never embedded")
	}
	fmt.Println()

	// GitHub API
	fmt.Println(bold("  GitHub API"))
	if result.RateLimit != nil {
		remainingStr := fmt.Sprintf("%d / %d", result.RateLimit.Remaining, result.RateLimit.Limit)
		fmt.Printf("    %s%15s\n", padLabel("Remaining:"), remainingStr)
		fmt.Printf("    %s%15s\n", padLabel("Resets:"), format.FormatRelativeTime(result.RateLimit.ResetAt))
	} else {
		fmt.Printf("    %s%15s\n", padLabel("Status:"), red("unavailable"))
	}
	fmt.Println()

	// Database
	fmt.Println(bold("  Database"))
	// Shorten path if it starts with home directory
	displayPath := result.DB.Path
	if home, err := os.UserHomeDir(); err == nil && len(displayPath) > len(home) {
		if displayPath[:len(home)] == home {
			displayPath = "~" + displayPath[len(home):]
		}
	}
	fmt.Printf("    %s%15s\n", padLabel("Path:"), displayPath)
	fmt.Printf("    %s%15s\n", padLabel("Size:"), result.DB.SizeMB)
	fmt.Println()

	fmt.Println(dim("─────────────────────────────────────────────"))
	timeStr := time.Now().Format("15:04:05")
	fmt.Println(dim("Generated at " + timeStr))

	return nil
}
