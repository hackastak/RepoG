package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/format"
	"github.com/hackastak/repog/internal/search"
)

// searchView is the Search tab: a query box over a scrollable results pane. It
// is a thin presentation layer over internal/search — SearchRepos does the
// work; this view only collects the query, renders results, and surfaces
// errors as UI state (never os.Exit, unlike commands/search.go).
type searchView struct {
	app *appContext

	input    textinput.Model
	spinner  spinner.Model
	viewport viewport.Model

	results   []search.SearchResult
	stats     string // timing summary for the results header
	lastQuery string

	searching    bool
	searched     bool // a search has completed at least once this session
	noEmbeddings bool // the corpus has no embeddings yet
	err          error

	lastContent string // cache so we only SetContent (and reset scroll) on change
}

// searchResultLimit is higher than the CLI's default of 3: a TUI pane has room
// to browse, and the results viewport scrolls.
const searchResultLimit = 10

func newSearchView(app *appContext) *searchView {
	ti := textinput.New()
	ti.Placeholder = "Search your repositories…"
	ti.Prompt = "🔍 "
	ti.CharLimit = 256
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = helpStyle

	return &searchView{
		app:     app,
		input:   ti,
		spinner: sp,
	}
}

func (v *searchView) Init() tea.Cmd {
	v.input.Focus()
	return textinput.Blink
}

// capturingText satisfies textInputView: while the query box is focused the
// root model must not steal plain keys (digits, "q", tab) for global actions.
func (v *searchView) capturingText() bool { return v.input.Focused() }

func (v *searchView) Update(msg tea.Msg) (view, tea.Cmd) {
	switch msg := msg.(type) {
	case searchDoneMsg:
		v.searching = false
		v.searched = true
		v.err = msg.err
		v.noEmbeddings = msg.noEmbeddings
		v.results = msg.result.Results
		v.lastQuery = msg.query
		v.stats = fmt.Sprintf("%dms embed · %dms search · %d chunks considered",
			msg.result.QueryEmbeddingMs, msg.result.SearchMs, msg.result.TotalConsidered)
		v.viewport.GotoTop()
		return v, nil

	case spinner.TickMsg:
		if !v.searching {
			return v, nil
		}
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			q := strings.TrimSpace(v.input.Value())
			if v.searching || q == "" {
				return v, nil
			}
			v.searching = true
			v.err = nil
			v.noEmbeddings = false
			return v, tea.Batch(v.spinner.Tick, runSearchCmd(v.app, q))
		case "esc":
			// Blur the query box so the global tab/quit keys work again, letting
			// the user browse results or leave the tab.
			v.input.Blur()
			return v, nil
		case "/", "i":
			if !v.input.Focused() {
				v.input.Focus()
				v.input.CursorEnd()
				return v, textinput.Blink
			}
		}

		if v.input.Focused() {
			var cmd tea.Cmd
			v.input, cmd = v.input.Update(msg)
			return v, cmd
		}
		// Browsing mode: keys scroll the results pane.
		var cmd tea.Cmd
		v.viewport, cmd = v.viewport.Update(msg)
		return v, cmd
	}

	return v, nil
}

func (v *searchView) View(width, height int) string {
	var b strings.Builder
	b.WriteString(v.input.View())
	b.WriteString("\n\n")

	switch {
	case v.searching:
		b.WriteString(v.spinner.View() + helpStyle.Render(" Searching…"))
	case v.err != nil:
		b.WriteString(errStyle.Render("Search failed: " + v.err.Error()))
	case v.noEmbeddings:
		b.WriteString(helpStyle.Render("No embeddings yet. Run a sync, then embed, from the Sync/Embed tab."))
	case v.searched && len(v.results) == 0:
		b.WriteString(helpStyle.Render("No matching repositories for " + strconv.Quote(v.lastQuery) + "."))
	case len(v.results) > 0:
		// The query box + blank line take two rows; leave the rest for results.
		v.viewport.Width = width
		v.viewport.Height = max(height-2, 1)
		content := v.renderResults(width)
		if content != v.lastContent {
			v.viewport.SetContent(content)
			v.lastContent = content
		}
		b.WriteString(v.viewport.View())
	default:
		b.WriteString(helpStyle.Render("Type a query and press Enter to search."))
	}

	return b.String()
}

func (v *searchView) HelpKeys() string {
	if v.input.Focused() {
		return "enter search · esc browse"
	}
	return "↑/↓ scroll · / edit query"
}

// renderResults formats the result list for the viewport. It reuses
// internal/format helpers so similarity/stars/truncation match the CLI output.
func (v *searchView) renderResults(width int) string {
	var b strings.Builder
	b.WriteString(helpStyle.Render(fmt.Sprintf("%d results · %s", len(v.results), v.stats)))
	b.WriteString("\n\n")

	descWidth := max(width-4, 20)
	for i, r := range v.results {
		name := titleStyle.Render(r.RepoFullName)
		sim := okStyle.Render(format.FormatSimilarity(r.Similarity))
		stars := helpStyle.Render("★ " + format.FormatStars(r.Stars))
		lang := ""
		if r.Language != "" {
			lang = helpStyle.Render("[" + r.Language + "]")
		}
		fmt.Fprintf(&b, "%d. %s  %s  %s  %s\n", i+1, name, sim, stars, lang)

		if r.Description != "" {
			b.WriteString("   " + helpStyle.Render(format.TruncateText(r.Description, descWidth)) + "\n")
		}
		b.WriteString("   " + helpStyle.Render(r.HTMLURL) + "\n")
		b.WriteString("   " + helpStyle.Render("["+r.ChunkType+"] ") + format.TruncateText(r.Content, 160) + "\n")

		if i < len(v.results)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// searchDoneMsg delivers the outcome of an async search to Update.
type searchDoneMsg struct {
	result       search.SearchQueryResult
	query        string
	noEmbeddings bool
	err          error
}

// runSearchCmd runs the vector search off the UI goroutine and returns the
// result as a tea.Msg. It reuses the shared embedding provider and the same
// search.SearchRepos the CLI calls — the only TUI-specific logic is the
// empty-corpus guard, surfaced as an empty-state rather than a hard error.
func runSearchCmd(app *appContext, query string) tea.Cmd {
	return func() tea.Msg {
		database := app.db()
		if database == nil {
			return searchDoneMsg{err: fmt.Errorf("not configured")}
		}

		// Empty-corpus guard (mirrors commands/search.go): no embeddings means
		// search can't return anything useful, so point the user at sync/embed.
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM chunk_embeddings").Scan(&count); err != nil {
			return searchDoneMsg{err: err}
		}
		if count == 0 {
			return searchDoneMsg{noEmbeddings: true}
		}

		embedProvider, err := app.embedProvider()
		if err != nil {
			return searchDoneMsg{err: err}
		}

		result, err := search.SearchRepos(context.Background(), database, embedProvider, query, search.SearchFilters{
			Limit: searchResultLimit,
		})
		if err != nil {
			return searchDoneMsg{err: err}
		}
		return searchDoneMsg{result: result, query: query}
	}
}
