package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/format"
	"github.com/hackastak/repog/internal/status"
)

type statusView struct {
	app    *appContext
	result *status.Result

	loading bool
	err     error
}

func newStatusView(app *appContext) *statusView {

	return &statusView{
		app:     app,
		loading: true,
	}
}

// statusLoadedMsg delivers the outcome of an async status load to Update.
// A tea.Msg is just an interface{}, so any struct can serve as a message.
type statusLoadedMsg struct {
	result status.Result
	err    error
}

// loadStatusCmd gathers the status snapshot off the UI goroutine: local stats
// via status.Collect plus the (network) GitHub rate limit. The returned func is
// a tea.Cmd — Bubbletea runs it in a goroutine and feeds its tea.Msg back into
// Update, which is how the Elm loop stays non-blocking.
func loadStatusCmd(app *appContext) tea.Cmd {
	return func() tea.Msg {
		db := app.db()
		if db == nil {
			return statusLoadedMsg{err: errors.New("no database available")}
		}

		ctx := context.Background()

		var dbPath string
		if app != nil && app.cfg != nil {
			dbPath = app.cfg.DBPath
		}

		result, err := status.Collect(ctx, db, dbPath)
		if err != nil {
			return statusLoadedMsg{err: err}
		}

		// Rate limit is best-effort; if the PAT is missing or the call fails it
		// stays nil and renders as "unavailable".
		if pat, patErr := config.GetGitHubPAT(); patErr == nil {
			result.RateLimit = status.FetchRateLimit(ctx, pat)
		}

		return statusLoadedMsg{result: result}
	}
}

// Init starts the status load when the view is first shown. rootModel also
// calls Init on each tab re-entry, so switching back to Status refreshes it.
func (v *statusView) Init() tea.Cmd {
	v.loading = true
	return loadStatusCmd(v.app)
}

// Update receives the load result and a manual reload key. The Status view has
// no interactive sub-components (no table/input), so anything else is a no-op.
func (v *statusView) Update(msg tea.Msg) (view, tea.Cmd) {
	switch msg := msg.(type) {
	case statusLoadedMsg:
		v.loading = false
		v.err = msg.err
		if msg.err == nil {
			// Address of the case-scoped copy; Go's escape analysis keeps it
			// alive on the heap, so the pointer is valid after we return.
			v.result = &msg.result
		}
		return v, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+r" {
			v.loading = true
			return v, loadStatusCmd(v.app)
		}
	}

	return v, nil
}

// View renders the status panel: a loading/error placeholder until the result
// arrives, then labeled sections built from v.result. width/height are unused
// for now (the panel is short and left-aligned); they matter once we add the
// language bars, which scale to the available width.
func (v *statusView) View(width, height int) string {
	if v.loading {
		return helpStyle.Render("Loading status…")
	}
	if v.err != nil {
		return errStyle.Render("Failed to load status: " + v.err.Error())
	}
	if v.result == nil {
		return helpStyle.Render("No status available.")
	}

	r := v.result
	var b strings.Builder

	// section writes a styled header; line writes an aligned label/value pair.
	section := func(title string) {
		b.WriteString("\n")
		b.WriteString(titleStyle.Render(title))
		b.WriteString("\n")
	}
	line := func(label, value string) {
		fmt.Fprintf(&b, "  %-16s %s\n", label+":", value)
	}

	section("Repositories")
	line("Total", strconv.Itoa(r.Repos.Total))
	line("Owned", strconv.Itoa(r.Repos.Owned))
	line("Starred", strconv.Itoa(r.Repos.Starred))
	line("Embedded", strconv.Itoa(r.Repos.EmbeddedCount))
	line("Pending embed", strconv.Itoa(r.Repos.PendingEmbed))

	if len(r.Languages) > 0 {
		section("Languages")
		b.WriteString(renderLanguageBars(r.Languages, width))
	}

	section("Knowledge Base")
	line("Chunks", strconv.Itoa(r.Embed.TotalChunks))
	line("Embeddings", strconv.Itoa(r.Embed.TotalEmbeddings))

	section("Last Sync")
	syncStatus := "Never synced"
	if r.Sync.LastSyncStatus != nil {
		syncStatus = *r.Sync.LastSyncStatus
	}
	line("Status", colorizeSyncStatus(syncStatus))
	if r.Sync.LastSyncedAt != nil {
		line("Date", format.FormatRelativeTime(*r.Sync.LastSyncedAt))
	}

	section("Last Embed")
	if r.Embed.LastEmbeddedAt != nil {
		line("Date", format.FormatRelativeTime(*r.Embed.LastEmbeddedAt))
	} else {
		line("Date", "Never embedded")
	}

	section("GitHub API")
	if r.RateLimit != nil {
		line("Remaining", fmt.Sprintf("%d / %d", r.RateLimit.Remaining, r.RateLimit.Limit))
		line("Resets", format.FormatRelativeTime(r.RateLimit.ResetAt))
	} else {
		line("Status", errStyle.Render("unavailable"))
	}

	section("Database")
	line("Path", r.DB.Path)
	line("Size", r.DB.SizeMB)

	return b.String()
}

// HelpKeys returns the contextual key hints shown in the bottom help line.
func (v *statusView) HelpKeys() string {
	return "ctrl+r reload"
}

// colorizeSyncStatus tints the sync status the same way the CLI does.
func colorizeSyncStatus(s string) string {
	switch s {
	case "completed":
		return okStyle.Render(s)
	case "failed":
		return errStyle.Render(s)
	default:
		return s
	}
}

// renderLanguageBars draws a horizontal percentage bar per language. The bar
// width scales with the available terminal width (clamped), which is why View
// now uses its width parameter. Only the top rows are shown; the rest collapse
// into an "…and N more" line.
func renderLanguageBars(langs []status.LanguageStat, width int) string {
	const maxRows = 8

	// Label column = the longest shown language name, capped.
	nameW := 8
	for i, l := range langs {
		if i >= maxRows {
			break
		}
		if n := len(l.Language); n > nameW {
			nameW = n
		}
	}
	if nameW > 16 {
		nameW = 16
	}

	// Bar scales with width, leaving room for the name and the trailing
	// "  100.0% (1234)" text; clamp to a sane range for narrow/wide terminals.
	barW := width - nameW - 18
	if barW < 10 {
		barW = 10
	}
	if barW > 40 {
		barW = 40
	}

	filledStyle := lipgloss.NewStyle().Foreground(colorAccent)
	emptyStyle := lipgloss.NewStyle().Foreground(colorMuted)

	var b strings.Builder
	for i, l := range langs {
		if i >= maxRows {
			fmt.Fprintf(&b, "  …and %d more\n", len(langs)-maxRows)
			break
		}
		filled := int(l.Percent/100*float64(barW) + 0.5) // round to nearest cell
		if filled > barW {
			filled = barW
		}
		bar := filledStyle.Render(strings.Repeat("█", filled)) +
			emptyStyle.Render(strings.Repeat("░", barW-filled))
		fmt.Fprintf(&b, "  %-*s %s %5.1f%% (%d)\n",
			nameW, truncateName(l.Language, nameW), bar, l.Percent, l.Count)
	}
	return b.String()
}

// truncateName shortens a language label that exceeds the column width,
// appending an ellipsis.
func truncateName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
