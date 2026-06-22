package tui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hackastak/repog/internal/recommend"
	"github.com/hackastak/repog/internal/summarize"
)

// reposView is the Repos tab: a scrollable, multi-selectable table of indexed
// repositories. recommend/summarize hang off the focused row (resolved
// decision: per-repo actions rather than separate tabs). Pressing "s"/"r" on a
// row swaps the table for a result pane (mode != reposModeTable); esc returns.
type reposView struct {
	app *appContext

	table    table.Model
	rows     []repoRow
	selected map[string]bool // full_name -> selected, for multi-select actions

	loading bool
	err     error
	ready   bool // columns/styles initialized

	// Per-repo action state (summarize/recommend). Only one action runs at a
	// time, so the fields are shared; opGen guards against stale stream/done
	// messages from a superseded action (mirrors askView.gen).
	mode      reposMode
	target    string // full_name the in-flight/last action operates on
	opGen     int
	busy      bool
	opErr     error
	spinner   spinner.Model
	viewport  viewport.Model
	opContent string // cached viewport content so we only SetContent on change

	// summarize streams its result token-by-token over a channel, like Ask.
	summary   string // accumulated streamed summary
	sumStream chan tea.Msg
	sumResult summarize.SummarizeResult

	// recommend resolves in a single call, like Search.
	recResult    recommend.RecommendResult
	noEmbeddings bool // recommend needs embeddings; surfaced as an empty-state
}

// Column widths. The repo-name column is computed from the available width;
// the rest are fixed.
const (
	colWSel      = 3
	colWLang     = 12
	colWStars    = 7
	colWState    = 12
	colWRepoMin  = 10
	colWRepoSeed = 40 // initial repo-name width before the first WindowSizeMsg
	colGutter    = 2  // bubbles/table cell padding per column
)

// reposColumns builds the table columns for a given repo-name width. Columns
// must exist before any SetRows call — table.renderRow indexes into them — so
// this is used at construction, not lazily in View.
func reposColumns(repoW int) []table.Column {
	if repoW < colWRepoMin {
		repoW = colWRepoMin
	}
	return []table.Column{
		{Title: "", Width: colWSel},
		{Title: "Repository", Width: repoW},
		{Title: "Language", Width: colWLang},
		{Title: "Stars", Width: colWStars},
		{Title: "State", Width: colWState},
	}
}

func newReposView(app *appContext) *reposView {
	t := table.New(
		table.WithFocused(true),
		table.WithColumns(reposColumns(colWRepoSeed)),
	)

	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).Foreground(colorAccent).
		BorderBottom(true).BorderForeground(colorMuted)
	st.Selected = st.Selected.Bold(true).
		Foreground(lipgloss.Color("231")).Background(colorAccent)
	t.SetStyles(st)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = helpStyle

	return &reposView{
		app:      app,
		table:    t,
		selected: make(map[string]bool),
		loading:  true,
		spinner:  sp,
	}
}

func (v *reposView) Init() tea.Cmd {
	v.loading = true
	return loadReposCmd(v.app.db())
}

func (v *reposView) Update(msg tea.Msg) (view, tea.Cmd) {
	switch msg := msg.(type) {
	case reposLoadedMsg:
		v.loading = false
		v.err = msg.err
		v.rows = msg.rows
		v.refreshRows()
		return v, nil

	// Action stream/done/spinner messages are handled the same way whatever the
	// active tab is, so a summary keeps streaming after a tab switch (they are
	// routedMsg). Delegated to updateAction so the key handling below stays lean.
	case summarizeChunkMsg, summarizeDoneMsg, recommendDoneMsg, spinner.TickMsg:
		return v.updateAction(msg)

	case tea.KeyMsg:
		// While a result pane is open, keys scroll it or dismiss it; the table
		// navigation/selection keys are inert until the user presses esc.
		if v.mode != reposModeTable {
			return v.updateActionKey(msg)
		}
		switch msg.String() {
		case " ":
			v.toggleSelected()
			return v, nil
		case "a":
			v.toggleAll()
			return v, nil
		case "s":
			return v.startSummarize()
		case "r":
			return v.startRecommend()
		case "ctrl+r":
			v.loading = true
			return v, loadReposCmd(v.app.db())
		}
	}

	var cmd tea.Cmd
	v.table, cmd = v.table.Update(msg)
	return v, cmd
}

func (v *reposView) View(width, height int) string {
	if v.loading {
		return helpStyle.Render("Loading repos…")
	}
	if v.err != nil {
		return errStyle.Render("Failed to load repos: " + v.err.Error())
	}
	if v.mode != reposModeTable {
		return v.renderAction(width, height)
	}
	if len(v.rows) == 0 {
		return titleStyle.Render("Repos") + "\n\n" +
			helpStyle.Render("No repositories indexed yet. Run a sync from the Sync/Embed tab.")
	}

	v.ensureLayout(width, height)

	var footer string
	if n := v.selectedCount(); n > 0 {
		footer = okStyle.Render(fmt.Sprintf("%d selected", n))
	} else {
		footer = helpStyle.Render(fmt.Sprintf("%d repos", len(v.rows)))
	}
	return v.table.View() + "\n" + footer
}

func (v *reposView) HelpKeys() string {
	if v.mode != reposModeTable {
		return "↑/↓ scroll · esc back"
	}
	return "↑/↓ move · space select · a all · s summarize · r recommend · ctrl+r reload"
}

// ensureLayout sizes the table to the available area and rebuilds columns so
// the repo-name column absorbs the remaining width.
func (v *reposView) ensureLayout(width, height int) {
	repoW := width - colWSel - colWLang - colWStars - colWState - (colGutter * 5)
	v.table.SetColumns(reposColumns(repoW))
	v.table.SetWidth(width)
	// Leave a row for the footer.
	if height > 1 {
		v.table.SetHeight(height - 1)
	}
	v.ready = true
}

// refreshRows rebuilds the table rows from v.rows + current selection.
// table.SetRows preserves the cursor (clamping only if it would overflow), so
// no manual cursor bookkeeping is needed.
func (v *reposView) refreshRows() {
	trows := make([]table.Row, 0, len(v.rows))
	for _, r := range v.rows {
		check := "[ ]"
		if v.selected[r.FullName] {
			check = "[x]"
		}
		lang := r.Language
		if lang == "" {
			lang = "—"
		}
		trows = append(trows, table.Row{
			check,
			r.FullName,
			lang,
			strconv.Itoa(r.Stars),
			repoState(r),
		})
	}
	v.table.SetRows(trows)
}

func (v *reposView) toggleSelected() {
	i := v.table.Cursor()
	if i < 0 || i >= len(v.rows) {
		return
	}
	name := v.rows[i].FullName
	if v.selected[name] {
		delete(v.selected, name)
	} else {
		v.selected[name] = true
	}
	v.refreshRows()
}

func (v *reposView) toggleAll() {
	if v.selectedCount() == len(v.rows) {
		v.selected = make(map[string]bool)
	} else {
		for _, r := range v.rows {
			v.selected[r.FullName] = true
		}
	}
	v.refreshRows()
}

func (v *reposView) selectedCount() int {
	n := 0
	for _, r := range v.rows {
		if v.selected[r.FullName] {
			n++
		}
	}
	return n
}

func repoState(r repoRow) string {
	switch {
	case r.Embedded:
		return "embedded ✓"
	case r.Synced:
		return "synced"
	default:
		return "—"
	}
}
