package tui

import (
	"database/sql"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hackastak/repog/internal/config"
)

// appContext holds the long-lived dependencies the views share. It is opened
// once when the TUI starts and passed to every view so they call the same
// internal/* packages the CLI commands do (no duplicated business logic).
type appContext struct {
	cfg *config.Config
	db  *sql.DB
}

// rootModel is the top-level Elm model. It owns the tab bar and window size and
// delegates Update/View to the active tab's view.
type rootModel struct {
	app *appContext

	active tab
	views  map[tab]view

	width  int
	height int

	// needsSetup is true when no config/credentials exist yet; the root opens
	// directly on the Settings/first-run screen in that case.
	needsSetup bool

	quitting bool
	err      error
}

// newRootModel builds the root model. A nil app means "not configured yet" —
// the UI starts on the first-run setup screen (built out in a later step).
func newRootModel(app *appContext, needsSetup bool) rootModel {
	m := rootModel{
		app:        app,
		needsSetup: needsSetup,
		views:      make(map[tab]view),
	}

	// Seed every tab with a placeholder view; real views replace these
	// incrementally in later steps.
	for _, t := range numberedTabs {
		m.views[t] = newPlaceholderView(t)
	}
	m.views[tabSettings] = newPlaceholderView(tabSettings)

	if needsSetup {
		m.active = tabSettings
	} else {
		m.active = tabRepos
	}
	return m
}

func (m rootModel) Init() tea.Cmd {
	return m.views[m.active].Init()
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Global keys are handled first; everything else is delegated to the
		// active view so it can own its own input (text fields, tables, etc.).
		switch msg.String() {
		case "ctrl+c", "q":
			// "q" quits only when the active view isn't capturing text input;
			// for now (placeholder views) it is always safe.
			m.quitting = true
			return m, tea.Quit
		case "tab":
			m.active = nextTab(m.active)
			return m, m.views[m.active].Init()
		case "shift+tab":
			m.active = prevTab(m.active)
			return m, m.views[m.active].Init()
		case "1", "2", "3", "4", "5":
			idx := int(msg.String()[0] - '1')
			if idx < len(numberedTabs) {
				m.active = numberedTabs[idx]
				return m, m.views[m.active].Init()
			}
		}
	}

	// Delegate to the active view.
	updated, cmd := m.views[m.active].Update(msg)
	m.views[m.active] = updated
	return m, cmd
}

func (m rootModel) View() string {
	if m.quitting {
		return ""
	}

	tabBar := m.renderTabBar()
	help := m.renderHelp()

	// Reserve vertical space for the tab bar and help line.
	innerWidth := m.width - appStyle.GetHorizontalFrameSize()
	innerHeight := m.height - lipgloss.Height(tabBar) - lipgloss.Height(help) - 1
	if innerWidth < 0 {
		innerWidth = 0
	}
	if innerHeight < 0 {
		innerHeight = 0
	}

	body := m.views[m.active].View(innerWidth, innerHeight)

	content := strings.Join([]string{tabBar, body, help}, "\n")
	return appStyle.Render(content)
}

// renderTabBar draws the numbered top tabs, highlighting the active one. When
// the Settings screen is active (not a numbered tab) it is appended as a hint.
func (m rootModel) renderTabBar() string {
	cells := make([]string, 0, len(numberedTabs))
	for i, t := range numberedTabs {
		label := tabLabel(i+1, t.title())
		if t == m.active {
			cells = append(cells, tabActiveStyle.Render(label))
		} else {
			cells = append(cells, tabInactiveStyle.Render(label))
		}
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top, cells...)
	if m.active == tabSettings {
		bar = lipgloss.JoinHorizontal(lipgloss.Top, bar, tabActiveStyle.Render("Settings"))
	}
	return tabBarStyle.Render(bar)
}

func (m rootModel) renderHelp() string {
	parts := []string{"tab/1-5 switch", "q quit"}
	if vk := m.views[m.active].HelpKeys(); vk != "" {
		parts = append([]string{vk}, parts...)
	}
	return helpStyle.Render(strings.Join(parts, " · "))
}

func tabLabel(num int, title string) string {
	return "[" + string(rune('0'+num)) + "] " + title
}

func nextTab(cur tab) tab {
	for i, t := range numberedTabs {
		if t == cur {
			return numberedTabs[(i+1)%len(numberedTabs)]
		}
	}
	// From Settings (non-numbered), tab returns to the first numbered tab.
	return numberedTabs[0]
}

func prevTab(cur tab) tab {
	for i, t := range numberedTabs {
		if t == cur {
			return numberedTabs[(i-1+len(numberedTabs))%len(numberedTabs)]
		}
	}
	return numberedTabs[len(numberedTabs)-1]
}
