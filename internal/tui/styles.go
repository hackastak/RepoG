// Package tui implements the interactive terminal UI for RepoG.
//
// See ADR-010 for why the TUI uses Bubbletea (Elm Model-Update-View).
// The TUI is a thin presentation layer over the existing internal/* packages
// (sync, embed, search, ask, recommend, summarize, status, config, db); it must
// not duplicate business logic. Resolved UX decisions (2026-06-12): tabbed
// dashboard navigation, all nine CLI commands surfaced, in-TUI first-run setup.
package tui

import "github.com/charmbracelet/lipgloss"

// Palette is intentionally small; lipgloss adapts colors to the terminal and
// honors NO_COLOR / non-color profiles automatically via termenv.
var (
	colorAccent = lipgloss.Color("63")  // violet — active/selection
	colorMuted  = lipgloss.Color("241") // dim gray — inactive/help
	colorErr    = lipgloss.Color("203") // red — errors
	colorOK     = lipgloss.Color("78")  // green — success
)

var (
	// appStyle frames the whole UI with a little breathing room.
	appStyle = lipgloss.NewStyle().Padding(0, 1)

	// tabActiveStyle / tabInactiveStyle render the top tab bar entries.
	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(colorAccent).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Padding(0, 1)

	tabBarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colorMuted)

	// helpStyle renders the bottom key hints.
	helpStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// titleStyle is used by placeholder views and headers.
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	errStyle = lipgloss.NewStyle().Foreground(colorErr)
	okStyle  = lipgloss.NewStyle().Foreground(colorOK)
)
