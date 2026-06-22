package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
)

// IsInteractive reports whether stdout and stdin are attached to a real
// terminal. The caller (bare `repog`) uses this to decide between launching the
// TUI and falling back to CLI/help — the TUI must never hang waiting for a TTY
// that isn't there (ADR-010 non-TTY fallback requirement).
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// Run starts the interactive TUI. It assumes IsInteractive() is true — the
// caller is responsible for the non-TTY fallback. When no config exists yet it
// opens directly on the first-run setup screen rather than erroring out.
func Run() error {
	app, needsSetup, err := loadAppContext()
	if err != nil {
		return err
	}

	p := tea.NewProgram(
		newRootModel(app, needsSetup),
		tea.WithAltScreen(),
	)
	finalModel, runErr := p.Run()

	// Close the database. When the user completes first-run setup or reconfigures
	// mid-session, the model adopts a freshly opened handle that differs from the
	// initial app, so close whatever the final model holds as well. Closing the
	// original too is harmless (a second Close is a no-op).
	if app != nil {
		if d := app.db(); d != nil {
			_ = d.Close()
		}
	}
	if rm, ok := finalModel.(rootModel); ok {
		if d := rm.app.db(); d != nil {
			_ = d.Close()
		}
	}
	return runErr
}

// loadAppContext loads config and opens the database. A missing/unconfigured
// install is not an error: it returns needsSetup=true with a nil app so the UI
// can drive the in-TUI first-run setup flow.
func loadAppContext() (app *appContext, needsSetup bool, err error) {
	if !config.IsConfigured() {
		return nil, true, nil
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		// Treat an unreadable/partial config as "needs setup" rather than a
		// hard failure, so the user can re-enter credentials from the TUI.
		return nil, true, nil //nolint:nilerr // intentional: fall through to setup
	}

	database, err := db.Open(cfg.DBPath, cfg.Embedding.Dimensions)
	if err != nil {
		return nil, false, fmt.Errorf("open database: %w", err)
	}

	return &appContext{cfg: cfg, database: database}, false, nil
}
