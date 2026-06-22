// Package commands provides CLI command implementations for repog.
package commands

import (
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/tui"
)

// version is set via ldflags during build
var version = "dev"

var rootCmd = &cobra.Command{
	Use:     "repog",
	Short:   "AI-powered knowledge base for your GitHub repositories",
	Version: version,
	Long: `RepoG is an AI-powered CLI tool that lets developers build a searchable
knowledge base from their GitHub repositories. It ingests repo metadata,
READMEs, and file trees; generates vector embeddings via Google Gemini;
and supports natural language search, Q&A, recommendations, and summarization.`,
	// Bare `repog` (no subcommand) launches the interactive TUI on a terminal.
	// On a non-TTY (piped output / CI) it must not hang waiting for input, so it
	// falls back to printing help. See ADR-010 for the non-TTY fallback rule.
	RunE: runRoot,
}

func runRoot(cmd *cobra.Command, args []string) error {
	if !tui.IsInteractive() {
		return cmd.Help()
	}
	return tui.Run()
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(reconfigCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(embedCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(recommendCmd)
	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(summarizeCmd)
	rootCmd.AddCommand(statusCmd)
}
