package tui

import tea "github.com/charmbracelet/bubbletea"

// view is the contract every tab's content implements. It mirrors the
// bubbletea.Model shape but is scoped to a single tab so the root model can
// delegate Update/View to the active tab. Views are built out incrementally in
// later steps; for now each tab uses placeholderView.
type view interface {
	// Init returns an optional initial command (e.g. a data load).
	Init() tea.Cmd
	// Update handles a message and returns the updated view + any command.
	Update(msg tea.Msg) (view, tea.Cmd)
	// View renders the body for the given inner width/height (already excludes
	// the tab bar and help line).
	View(width, height int) string
	// HelpKeys returns short contextual key hints for the bottom help line.
	HelpKeys() string
}

// textInputView is an optional interface implemented by views that capture
// free-text input (e.g. Search, Ask). The root model consults it before
// treating plain keys as global shortcuts, so a focused text field isn't robbed
// of "q", digits, or tab — only ctrl+c stays global while text is being typed.
type textInputView interface {
	capturingText() bool
}

// placeholderView is a stand-in used while the real views are built out. It
// renders the tab title and a "coming soon" note so the shell is navigable and
// testable before any view logic lands.
type placeholderView struct {
	tab tab
}

func newPlaceholderView(t tab) *placeholderView { return &placeholderView{tab: t} }

func (p *placeholderView) Init() tea.Cmd { return nil }

func (p *placeholderView) Update(tea.Msg) (view, tea.Cmd) { return p, nil }

func (p *placeholderView) View(width, height int) string {
	header := titleStyle.Render(p.tab.title())
	body := helpStyle.Render("This view is not implemented yet.")
	return header + "\n\n" + body
}

func (p *placeholderView) HelpKeys() string { return "" }
