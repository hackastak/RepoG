package tui

// tab identifies a primary view in the tabbed dashboard.
//
// The five primary tabs map to the MVP views (PRD §4.3). recommend/summarize
// are reached as per-repo actions from the Repos tab, and init/reconfig live in
// a Settings screen (tabSettings) that is reachable by key rather than as a
// numbered tab.
type tab int

const (
	tabRepos tab = iota
	tabSearch
	tabAsk
	tabSync
	tabStatus

	// tabSettings is not part of the numbered tab bar; it is opened explicitly
	// and also hosts the first-run setup flow.
	tabSettings
)

// numberedTabs are the tabs shown in the top bar, in display order. Their
// position here defines the 1..N hotkeys.
var numberedTabs = []tab{tabRepos, tabSearch, tabAsk, tabSync, tabStatus}

func (t tab) title() string {
	switch t {
	case tabRepos:
		return "Repos"
	case tabSearch:
		return "Search"
	case tabAsk:
		return "Ask"
	case tabSync:
		return "Sync/Embed"
	case tabStatus:
		return "Status"
	case tabSettings:
		return "Settings"
	default:
		return "?"
	}
}
