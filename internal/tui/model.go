package tui

import (
	"database/sql"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/provider"

	// Register every provider so the embedding and generation providers resolve
	// for whatever the user configured (Search needs embeddings, Ask also needs
	// generation). Mirrors the blank imports in commands/init.go.
	_ "github.com/hackastak/repog/internal/provider/anthropic"
	_ "github.com/hackastak/repog/internal/provider/gemini"
	_ "github.com/hackastak/repog/internal/provider/ollama"
	_ "github.com/hackastak/repog/internal/provider/openai"
	_ "github.com/hackastak/repog/internal/provider/openrouter"
	_ "github.com/hackastak/repog/internal/provider/voyageai"
)

// appContext holds the long-lived dependencies the views share. It is opened
// once when the TUI starts and passed to every view so they call the same
// internal/* packages the CLI commands do (no duplicated business logic).
type appContext struct {
	cfg      *config.Config
	database *sql.DB

	// embed is the lazily-built embedding provider shared by Search and Ask.
	// Use embedProvider() rather than touching this directly.
	embed provider.EmbeddingProvider

	// llm is the lazily-built generation provider used by Ask. Use llmProvider()
	// rather than touching this directly.
	llm provider.LLMProvider
}

// db returns the shared database handle, or nil when the install isn't
// configured yet. It is nil-safe on the receiver so views can call it
// unconditionally during the first-run/setup state.
func (a *appContext) db() *sql.DB {
	if a == nil {
		return nil
	}
	return a.database
}

// embedProvider lazily constructs and caches the embedding provider that the
// Search and Ask views need. Construction is deferred (keyring/API-key access)
// until a feature first requires it, so the TUI still launches when the install
// is only partially configured. The cache lives on the shared *appContext, so
// Search and Ask reuse one instance.
func (a *appContext) embedProvider() (provider.EmbeddingProvider, error) {
	if a == nil || a.cfg == nil {
		return nil, fmt.Errorf("not configured")
	}
	if a.embed != nil {
		return a.embed, nil
	}
	apiKey, err := config.GetAPIKeyForProvider(a.cfg.Embedding.Provider)
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}
	p, err := provider.NewEmbeddingProvider(a.cfg.Embedding, apiKey)
	if err != nil {
		return nil, fmt.Errorf("create embedding provider: %w", err)
	}
	a.embed = p
	return p, nil
}

// llmProvider lazily constructs and caches the generation (LLM) provider the Ask
// view needs. Like embedProvider, construction is deferred until first use so the
// TUI still launches on a partially-configured install, and the result is cached
// on the shared *appContext.
func (a *appContext) llmProvider() (provider.LLMProvider, error) {
	if a == nil || a.cfg == nil {
		return nil, fmt.Errorf("not configured")
	}
	if a.llm != nil {
		return a.llm, nil
	}
	apiKey, err := config.GetAPIKeyForProvider(a.cfg.Generation.Provider)
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}
	p, err := provider.NewLLMProvider(a.cfg.Generation, apiKey)
	if err != nil {
		return nil, fmt.Errorf("create generation provider: %w", err)
	}
	a.llm = p
	return p, nil
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

	// Real views (replacing placeholders as they land).
	m.views[tabRepos] = newReposView(app)
	m.views[tabSearch] = newSearchView(app)
	m.views[tabAsk] = newAskView(app)
	m.views[tabSync] = newSyncView(app)
	m.views[tabStatus] = newStatusView(app)

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
	// Streaming progress messages are delivered to the view that owns them even
	// when another tab is active, so a long-running sync/embed or a streaming Ask
	// answer keeps advancing after the user switches tabs. The owning view's
	// Update re-issues the wait command that drives the stream, so the delivery
	// has to land there rather than on m.active.
	if rm, ok := msg.(routedMsg); ok {
		target := rm.targetTab()
		updated, cmd := m.views[target].Update(msg)
		m.views[target] = updated
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// ctrl+c always quits. The other global keys (q to quit, tab/1-5 to
		// switch) are suppressed while the active view is capturing free text,
		// so typing a query doesn't quit the app or jump tabs. Everything else
		// is delegated to the active view so it owns its own input.
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if !m.activeCapturesText() {
			switch msg.String() {
			case "q":
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
	var parts []string
	if vk := m.views[m.active].HelpKeys(); vk != "" {
		parts = append(parts, vk)
	}
	// While a text field is focused the global switch/quit keys are inert
	// (they'd be typed into the field), so advertise only what actually works.
	if m.activeCapturesText() {
		parts = append(parts, "ctrl+c quit")
	} else {
		parts = append(parts, "tab/1-5 switch", "q quit")
	}
	return helpStyle.Render(strings.Join(parts, " · "))
}

// activeCapturesText reports whether the active view is currently capturing
// free-text input, so the root model knows to defer plain keys to it.
func (m rootModel) activeCapturesText() bool {
	if tv, ok := m.views[m.active].(textInputView); ok {
		return tv.capturingText()
	}
	return false
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
