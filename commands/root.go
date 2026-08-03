// Package commands provides CLI command implementations for repog.
package commands

import (
	"context"
	"os"
	"os/signal"
	"syscall"

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

// commandContext returns cmd's context, falling back to context.Background when
// none was set. cobra only populates the context when a command runs via
// ExecuteContext (the real CLI path, wired in Execute). A unit test that invokes
// a RunE directly gets a nil context, which would panic the first database/sql
// or HTTP call — this keeps those call sites safe either way.
func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func runRoot(cmd *cobra.Command, args []string) error {
	if !tui.IsInteractive() {
		return cmd.Help()
	}
	return tui.Run()
}

// Execute runs the root command.
//
// It installs a context that is cancelled on the first SIGINT/SIGTERM, so a
// Ctrl-C during a long sync or embed stops in-flight HTTP requests and lets the
// pipelines unwind cleanly instead of killing the process mid-write. A second
// signal restores the default behaviour (immediate termination). Subcommands
// reach this context through cmd.Context().
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return rootCmd.ExecuteContext(ctx)
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
