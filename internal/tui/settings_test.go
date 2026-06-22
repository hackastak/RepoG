package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/config"
)

// typeRunes feeds a string into the focused text input as a single rune key.
func typeRunes(v *settingsView, s string) {
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

func key(v *settingsView, t tea.KeyType) (view, tea.Cmd) {
	return v.Update(tea.KeyMsg{Type: t})
}

// TestSettingsFirstRunDetected confirms a nil/unconfigured app context puts the
// view into first-run mode starting on the GitHub token step, capturing keys.
func TestSettingsFirstRunDetected(t *testing.T) {
	v := newSettingsView(nil)
	if !v.firstRun {
		t.Fatal("expected firstRun=true for a nil app context")
	}
	if v.step != stepGitHubPAT {
		t.Fatalf("expected to start on stepGitHubPAT, got %v", v.step)
	}
	if !v.capturingText() {
		t.Fatal("expected the setup flow to capture keys so globals don't interrupt it")
	}
}

// TestSettingsEmptyTokenRejected confirms submitting an empty token surfaces an
// error and stays on the same step rather than advancing.
func TestSettingsEmptyTokenRejected(t *testing.T) {
	v := newSettingsView(nil)
	key(v, tea.KeyEnter)
	if v.err != errEmptyToken {
		t.Fatalf("expected errEmptyToken, got %v", v.err)
	}
	if v.step != stepGitHubPAT {
		t.Fatalf("expected to stay on stepGitHubPAT, got %v", v.step)
	}
}

// TestSettingsPATSubmitGoesBusy confirms a non-empty token submission starts an
// async validation (busy) without advancing until the result arrives.
func TestSettingsPATSubmitGoesBusy(t *testing.T) {
	v := newSettingsView(nil)
	typeRunes(v, "ghp_token")
	updated, cmd := key(v, tea.KeyEnter)
	v = updated.(*settingsView)
	if !v.busy {
		t.Fatal("expected busy=true while the token validates")
	}
	if cmd == nil {
		t.Fatal("expected a validation command to be returned")
	}
	if v.githubPAT != "ghp_token" {
		t.Fatalf("expected the token to be captured, got %q", v.githubPAT)
	}
}

// TestSettingsFullGeminiFlow walks the happy path end to end (feeding validation
// results directly, no network), confirming each step advances and that saving
// hands a new app context back to the root via settingsDoneMsg.
func TestSettingsFullGeminiFlow(t *testing.T) {
	v := newSettingsView(nil)

	// GitHub token → validated.
	typeRunes(v, "ghp_token")
	key(v, tea.KeyEnter)
	v.Update(patValidatedMsg{login: "octocat"})
	if v.step != stepEmbedProvider {
		t.Fatalf("after PAT validation expected stepEmbedProvider, got %v", v.step)
	}
	if v.login != "octocat" {
		t.Fatalf("expected login to be recorded, got %q", v.login)
	}

	// Embedding provider: gemini is the first option (cursor 0). Choose it.
	key(v, tea.KeyEnter)
	if v.step != stepEmbedKey {
		t.Fatalf("expected stepEmbedKey for gemini, got %v", v.step)
	}
	if v.embedProv != "gemini" || v.embedCfg.Provider != "gemini" {
		t.Fatalf("expected gemini embedding config, got %q/%q", v.embedProv, v.embedCfg.Provider)
	}

	// Embedding key → validated.
	typeRunes(v, "embed-key")
	key(v, tea.KeyEnter)
	v.Update(embedValidatedMsg{})
	if v.step != stepGenProvider {
		t.Fatalf("expected stepGenProvider, got %v", v.step)
	}

	// Generation provider: gemini again → matches embedding, so reuse is offered.
	key(v, tea.KeyEnter)
	if v.step != stepGenReuse {
		t.Fatalf("expected stepGenReuse when gen provider matches embed, got %v", v.step)
	}

	// Reuse the embedding key (first option) → validated.
	key(v, tea.KeyEnter)
	if v.genKey != v.embedKey {
		t.Fatalf("expected the embedding key to be reused, got %q vs %q", v.genKey, v.embedKey)
	}
	v.Update(genValidatedMsg{})
	if v.step != stepDBPath {
		t.Fatalf("expected stepDBPath, got %v", v.step)
	}

	// Database path: accept the default → save starts.
	updated, _ := key(v, tea.KeyEnter)
	v = updated.(*settingsView)
	if !v.busy {
		t.Fatal("expected save to start (busy) after the db path step")
	}

	// Simulate a successful save; the view should emit settingsDoneMsg carrying
	// the new app context.
	app := &appContext{}
	updated, cmd := v.Update(settingsSavedMsg{app: app})
	v = updated.(*settingsView)
	if v.step != stepDone {
		t.Fatalf("expected stepDone after a successful save, got %v", v.step)
	}
	if cmd == nil {
		t.Fatal("expected a command emitting settingsDoneMsg")
	}
	done, ok := cmd().(settingsDoneMsg)
	if !ok {
		t.Fatalf("expected settingsDoneMsg, got %T", cmd())
	}
	if done.app != app {
		t.Fatal("expected settingsDoneMsg to carry the saved app context")
	}
}

// TestSettingsOllamaSkipsKey confirms picking ollama (local, no API key) skips
// the key step and goes straight to validation.
func TestSettingsOllamaSkipsKey(t *testing.T) {
	v := newSettingsView(nil)
	typeRunes(v, "ghp_token")
	key(v, tea.KeyEnter)
	v.Update(patValidatedMsg{login: "octocat"})

	// Move the cursor to ollama (last embedding option) and select it.
	for v.cursor < len(embedProviderOptions)-1 {
		key(v, tea.KeyDown)
	}
	if embedProviderOptions[v.cursor] != "ollama" {
		t.Fatalf("expected cursor on ollama, got %q", embedProviderOptions[v.cursor])
	}
	updated, cmd := key(v, tea.KeyEnter)
	v = updated.(*settingsView)
	if v.embedKey != "" {
		t.Fatalf("expected no API key for ollama, got %q", v.embedKey)
	}
	if !v.busy {
		t.Fatal("expected validation to start immediately for ollama (no key step)")
	}
	if cmd == nil {
		t.Fatal("expected a validation command for ollama")
	}
}

// TestSettingsValidationErrorStays confirms a failed credential check surfaces
// the error and keeps the user on the step to retry.
func TestSettingsValidationErrorStays(t *testing.T) {
	v := newSettingsView(nil)
	typeRunes(v, "ghp_token")
	key(v, tea.KeyEnter)

	v.Update(patValidatedMsg{err: errEmptyToken}) // any non-nil error
	if v.busy {
		t.Fatal("expected busy to clear after a validation result")
	}
	if v.err == nil {
		t.Fatal("expected the validation error to be surfaced")
	}
	if v.step != stepGitHubPAT {
		t.Fatalf("expected to stay on stepGitHubPAT after a failed PAT check, got %v", v.step)
	}
}

// TestSettingsReconfigCanCancel confirms that when an install is already
// configured (not first-run), esc cancels back to the tabs.
func TestSettingsReconfigCanCancel(t *testing.T) {
	v := configuredSettingsView()
	if v.firstRun {
		t.Fatal("expected a configured app context to disable first-run mode")
	}
	_, cmd := key(v, tea.KeyEsc)
	if cmd == nil {
		t.Fatal("expected esc to emit a cancel command in reconfig mode")
	}
	if _, ok := cmd().(settingsCancelledMsg); !ok {
		t.Fatalf("expected settingsCancelledMsg, got %T", cmd())
	}
}

// TestSettingsFirstRunCannotCancel confirms esc is inert during first-run (there
// is nowhere to return to).
func TestSettingsFirstRunCannotCancel(t *testing.T) {
	v := newSettingsView(nil)
	_, cmd := key(v, tea.KeyEsc)
	if cmd != nil {
		t.Fatal("expected esc to be inert during first-run setup")
	}
}

// configuredSettingsView builds a settings view whose app context looks
// configured, so firstRun is false.
func configuredSettingsView() *settingsView {
	cfg := &config.Config{
		Embedding:  config.DefaultEmbeddingConfig(),
		Generation: config.DefaultGenerationConfig(),
	}
	return newSettingsView(&appContext{cfg: cfg})
}

// TestRootOpensSettingsOnFirstRun confirms a needs-setup launch lands on the
// Settings tab.
func TestRootOpensSettingsOnFirstRun(t *testing.T) {
	m := newRootModel(nil, true)
	if m.active != tabSettings {
		t.Fatalf("expected first-run to open on Settings, got %v", m.active)
	}
}

// TestRootSettingsDoneAdoptsApp confirms completing setup installs the new app
// context, clears needsSetup, and switches to the Repos tab.
func TestRootSettingsDoneAdoptsApp(t *testing.T) {
	m := newRootModel(nil, true)
	app := &appContext{cfg: &config.Config{
		Embedding:  config.DefaultEmbeddingConfig(),
		Generation: config.DefaultGenerationConfig(),
	}}

	updated, _ := m.Update(settingsDoneMsg{app: app})
	m = updated.(rootModel)

	if m.app != app {
		t.Fatal("expected the new app context to be adopted")
	}
	if m.needsSetup {
		t.Fatal("expected needsSetup to be cleared after setup completes")
	}
	if m.active != tabRepos {
		t.Fatalf("expected to land on Repos after setup, got %v", m.active)
	}
	// The Settings view should have been rebuilt against the new (configured)
	// app, so it is no longer in first-run mode.
	if sv, ok := m.views[tabSettings].(*settingsView); ok && sv.firstRun {
		t.Fatal("expected the rebuilt Settings view to leave first-run mode")
	}
}

// TestRootSettingsKeyOpensSettings confirms the global "S" key opens the
// Settings screen from a numbered tab.
func TestRootSettingsKeyOpensSettings(t *testing.T) {
	m := newRootModel(&appContext{}, false)
	if m.active != tabRepos {
		t.Fatalf("expected to start on Repos, got %v", m.active)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = updated.(rootModel)
	if m.active != tabSettings {
		t.Fatalf("expected 'S' to open Settings, got %v", m.active)
	}
}

// TestRootSettingsCancelReturnsToRepos confirms cancelling reconfig returns to
// the Repos tab.
func TestRootSettingsCancelReturnsToRepos(t *testing.T) {
	m := newRootModel(&appContext{}, false)
	m.active = tabSettings
	updated, _ := m.Update(settingsCancelledMsg{})
	m = updated.(rootModel)
	if m.active != tabRepos {
		t.Fatalf("expected cancel to return to Repos, got %v", m.active)
	}
}

// TestSettingsViewRendersStep confirms the body renders the current step's
// prompt so the screen is not blank.
func TestSettingsViewRendersStep(t *testing.T) {
	v := newSettingsView(nil)
	out := v.View(80, 24)
	for _, want := range []string{"first-run setup", "GitHub", "Step 1 of 4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected setup view to contain %q, got:\n%s", want, out)
		}
	}
}
