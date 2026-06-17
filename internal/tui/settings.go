package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/github"
	"github.com/hackastak/repog/internal/provider"
)

// settingsView is the Settings screen (ADR-010 UX decision #2/#3). It is not a
// numbered tab: the root model opens it directly on first run (no config yet)
// and on demand via the global "S" key for reconfiguration. The same screen
// serves both — its "empty state" is the first-run setup flow.
//
// It walks the user through the same steps as `repog init`/`reconfig` (GitHub
// PAT → embedding provider → generation provider → database path), validating
// each credential against the live provider before saving, then writes the
// config file and keyring entries via internal/config. It is a thin layer over
// internal/{config,db,github,provider}: no business logic is duplicated, and
// failures surface as UI state rather than os.Exit (unlike commands/init.go).
//
// On success it emits settingsDoneMsg with a freshly-built *appContext; the root
// model adopts it, rebuilds every view against the new config, and drops the
// user on the Repos tab.
type settingsView struct {
	app      *appContext
	firstRun bool // no usable config existed when this view was built

	step    setupStep
	input   textinput.Model // reused across the text steps (PAT, keys, db path)
	spinner spinner.Model
	cursor  int // highlighted option on selection steps

	busy    bool   // an async validation/save is in flight
	busyMsg string // label shown next to the spinner while busy
	err     error  // last validation/save error, shown until the next action

	// Collected as the flow progresses.
	githubPAT string
	login     string // GitHub handle from a successful PAT validation
	embedProv string
	embedCfg  config.ProviderConfig
	embedKey  string
	genProv   string
	genCfg    config.ProviderConfig
	genKey    string
	dbPath    string
}

// setupStep enumerates the input steps of the setup flow. Validation/save are
// represented by the busy flag rather than dedicated steps, so an error returns
// the user to the step they were on.
type setupStep int

const (
	stepGitHubPAT setupStep = iota
	stepEmbedProvider
	stepEmbedKey
	stepGenProvider
	stepGenReuse // offered only when the gen provider matches the embed provider
	stepGenKey
	stepDBPath
	stepDone
)

// Provider option lists mirror commands/init.go (embedding excludes anthropic;
// generation excludes voyageai).
var (
	embedProviderOptions = []string{"gemini", "openai", "openrouter", "voyageai", "ollama"}
	genProviderOptions   = []string{"gemini", "openai", "openrouter", "anthropic", "ollama"}
	reuseKeyOptions      = []string{"Use the same API key", "Enter a different key"}
)

var (
	errEmptyToken = errors.New("a GitHub token is required")
	errEmptyKey   = errors.New("an API key is required")
)

func newSettingsView(app *appContext) *settingsView {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.CharLimit = 512

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = helpStyle

	v := &settingsView{
		app:      app,
		firstRun: app == nil || app.cfg == nil,
		input:    ti,
		spinner:  sp,
	}
	v.reset()
	return v
}

// Init resets the flow so re-opening Settings (or launching into first-run)
// always starts clean. The root model only switches into this view via "S" or
// first-run, never mid-flow, so resetting here is safe.
func (v *settingsView) Init() tea.Cmd {
	v.reset()
	return textinput.Blink
}

// capturingText keeps every key with the Settings view for the whole flow, so
// the global tab/quit shortcuts can't interrupt credential entry. Only ctrl+c
// stays global (handled by the root model). Once setup is done, keys are released.
func (v *settingsView) capturingText() bool { return v.step != stepDone }

func (v *settingsView) Update(msg tea.Msg) (view, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !v.busy {
			return v, nil
		}
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd

	case patValidatedMsg:
		v.busy = false
		if msg.err != nil {
			v.err = msg.err
			return v, nil
		}
		v.login = msg.login
		v.enterStep(stepEmbedProvider)
		return v, nil

	case embedValidatedMsg:
		v.busy = false
		if msg.err != nil {
			v.err = msg.err // stay on the current step so the user can fix it
			return v, nil
		}
		v.enterStep(stepGenProvider)
		return v, nil

	case genValidatedMsg:
		v.busy = false
		if msg.err != nil {
			v.err = msg.err
			return v, nil
		}
		v.enterStep(stepDBPath)
		return v, textinput.Blink

	case settingsSavedMsg:
		v.busy = false
		if msg.err != nil {
			v.err = msg.err
			return v, nil
		}
		v.step = stepDone
		app := msg.app
		return v, func() tea.Msg { return settingsDoneMsg{app: app} }

	case tea.KeyMsg:
		if v.busy {
			// Ignore input while a validation/save is running; ctrl+c is handled
			// globally by the root model.
			return v, nil
		}
		return v.handleKey(msg)
	}
	return v, nil
}

func (v *settingsView) handleKey(msg tea.KeyMsg) (view, tea.Cmd) {
	if msg.String() == "esc" {
		// First-run has nowhere to go back to; reconfig returns to the tabs.
		if v.firstRun {
			return v, nil
		}
		return v, func() tea.Msg { return settingsCancelledMsg{} }
	}

	if opts := v.currentOptions(); opts != nil {
		switch msg.String() {
		case "up", "k":
			if v.cursor > 0 {
				v.cursor--
			}
		case "down", "j":
			if v.cursor < len(opts)-1 {
				v.cursor++
			}
		case "enter":
			return v.chooseOption()
		}
		return v, nil
	}

	// Text step.
	if msg.String() == "enter" {
		return v.submitText()
	}
	var cmd tea.Cmd
	v.input, cmd = v.input.Update(msg)
	return v, cmd
}

// chooseOption handles Enter on a selection step.
func (v *settingsView) chooseOption() (view, tea.Cmd) {
	switch v.step {
	case stepEmbedProvider:
		v.embedProv = embedProviderOptions[v.cursor]
		v.embedCfg = defaultEmbedConfigFor(v.embedProv)
		if v.embedProv == "ollama" { // local; no API key
			v.embedKey = ""
			return v, v.startValidateEmbed()
		}
		v.enterStep(stepEmbedKey)
		return v, textinput.Blink
	case stepGenProvider:
		v.genProv = genProviderOptions[v.cursor]
		v.genCfg = defaultGenConfigFor(v.genProv)
		if v.genProv == "ollama" {
			v.genKey = ""
			return v, v.startValidateGen()
		}
		if v.genProv == v.embedProv && v.embedKey != "" {
			v.enterStep(stepGenReuse)
			return v, nil
		}
		v.enterStep(stepGenKey)
		return v, textinput.Blink
	case stepGenReuse:
		if v.cursor == 0 { // reuse the embedding key
			v.genKey = v.embedKey
			return v, v.startValidateGen()
		}
		v.enterStep(stepGenKey)
		return v, textinput.Blink
	}
	return v, nil
}

// submitText handles Enter on a text-input step.
func (v *settingsView) submitText() (view, tea.Cmd) {
	val := strings.TrimSpace(v.input.Value())
	switch v.step {
	case stepGitHubPAT:
		if val == "" {
			v.err = errEmptyToken
			return v, nil
		}
		v.githubPAT = val
		v.busy = true
		v.busyMsg = "Validating GitHub token…"
		v.err = nil
		return v, tea.Batch(v.spinner.Tick, validatePATCmd(val))
	case stepEmbedKey:
		if val == "" {
			v.err = errEmptyKey
			return v, nil
		}
		v.embedKey = val
		return v, v.startValidateEmbed()
	case stepGenKey:
		if val == "" {
			v.err = errEmptyKey
			return v, nil
		}
		v.genKey = val
		return v, v.startValidateGen()
	case stepDBPath:
		if val == "" {
			val = config.DefaultDBPath()
		}
		v.dbPath = val
		v.busy = true
		v.busyMsg = "Saving configuration…"
		v.err = nil
		return v, tea.Batch(v.spinner.Tick, v.save())
	}
	return v, nil
}

func (v *settingsView) startValidateEmbed() tea.Cmd {
	v.busy = true
	v.busyMsg = fmt.Sprintf("Validating %s embedding…", v.embedProv)
	v.err = nil
	return tea.Batch(v.spinner.Tick, validateEmbedCmd(v.embedCfg, v.embedKey))
}

func (v *settingsView) startValidateGen() tea.Cmd {
	v.busy = true
	v.busyMsg = fmt.Sprintf("Validating %s generation…", v.genProv)
	v.err = nil
	return tea.Batch(v.spinner.Tick, validateGenCmd(v.genCfg, v.genKey))
}

// enterStep transitions to s and prepares its input/cursor.
func (v *settingsView) enterStep(s setupStep) {
	v.step = s
	v.err = nil
	v.cursor = 0
	switch s {
	case stepGitHubPAT:
		v.configureInput("GitHub Personal Access Token", true, "")
	case stepEmbedKey:
		v.configureInput(fmt.Sprintf("%s API key", v.embedProv), true, "")
	case stepGenKey:
		v.configureInput(fmt.Sprintf("%s API key", v.genProv), true, "")
	case stepDBPath:
		v.configureInput("Database path", false, config.DefaultDBPath())
	case stepEmbedProvider:
		v.cursor = indexOf(embedProviderOptions, v.defaultProvider(embedProviderOptions))
	case stepGenProvider:
		v.cursor = indexOf(genProviderOptions, v.defaultGenProvider())
	}
}

// configureInput points the shared text input at the current step.
func (v *settingsView) configureInput(placeholder string, secret bool, value string) {
	v.input.SetValue(value)
	v.input.Placeholder = placeholder
	if secret {
		v.input.EchoMode = textinput.EchoPassword
	} else {
		v.input.EchoMode = textinput.EchoNormal
	}
	v.input.CursorEnd()
	v.input.Focus()
}

// reset returns the flow to its first step, clearing any collected secrets.
func (v *settingsView) reset() {
	v.busy = false
	v.err = nil
	v.login = ""
	v.githubPAT = ""
	v.embedProv, v.embedKey, v.embedCfg = "", "", config.ProviderConfig{}
	v.genProv, v.genKey, v.genCfg = "", "", config.ProviderConfig{}
	v.dbPath = ""
	v.enterStep(stepGitHubPAT)
}

// defaultProvider pre-selects the existing embedding provider on reconfig, else
// the first option (gemini).
func (v *settingsView) defaultProvider(opts []string) string {
	if v.app != nil && v.app.cfg != nil && v.app.cfg.Embedding.Provider != "" {
		return v.app.cfg.Embedding.Provider
	}
	return opts[0]
}

func (v *settingsView) defaultGenProvider() string {
	if v.app != nil && v.app.cfg != nil && v.app.cfg.Generation.Provider != "" {
		return v.app.cfg.Generation.Provider
	}
	return genProviderOptions[0]
}

// currentOptions returns the option list for a selection step, or nil for the
// text-input steps.
func (v *settingsView) currentOptions() []string {
	switch v.step {
	case stepEmbedProvider:
		return embedProviderOptions
	case stepGenProvider:
		return genProviderOptions
	case stepGenReuse:
		return reuseKeyOptions
	default:
		return nil
	}
}

func (v *settingsView) View(width, height int) string {
	var b strings.Builder
	if v.firstRun {
		b.WriteString(titleStyle.Render("Welcome to RepoG — first-run setup"))
	} else {
		b.WriteString(titleStyle.Render("Settings"))
	}
	b.WriteString("\n")

	line := v.progressLine()
	if v.login != "" {
		line += helpStyle.Render("  ·  ") + okStyle.Render("@"+v.login)
	}
	b.WriteString(helpStyle.Render(v.progressLabel()) + "  " + line)
	b.WriteString("\n\n")

	if v.busy {
		b.WriteString(v.spinner.View() + helpStyle.Render(" "+v.busyMsg))
		return b.String()
	}

	b.WriteString(v.stepBody())

	if v.err != nil {
		b.WriteString("\n\n" + errStyle.Render("✗ "+v.err.Error()))
	}
	return b.String()
}

func (v *settingsView) HelpKeys() string {
	if v.busy {
		return "working…"
	}
	keys := "enter continue"
	if v.currentOptions() != nil {
		keys = "↑/↓ select · enter continue"
	}
	if !v.firstRun {
		keys += " · esc cancel"
	}
	return keys
}

// progressLabel renders a compact "Step N of 4" marker; the embedding/generation
// sub-steps collapse into a single phase each.
func (v *settingsView) progressLabel() string {
	phase := 1
	switch v.step {
	case stepEmbedProvider, stepEmbedKey:
		phase = 2
	case stepGenProvider, stepGenReuse, stepGenKey:
		phase = 3
	case stepDBPath, stepDone:
		phase = 4
	}
	return fmt.Sprintf("Step %d of 4", phase)
}

func (v *settingsView) progressLine() string {
	switch v.step {
	case stepGitHubPAT:
		return "GitHub token"
	case stepEmbedProvider, stepEmbedKey:
		return "Embedding provider"
	case stepGenProvider, stepGenReuse, stepGenKey:
		return "Generation provider"
	case stepDBPath:
		return "Database"
	case stepDone:
		return "Done"
	}
	return ""
}

func (v *settingsView) stepBody() string {
	switch v.step {
	case stepGitHubPAT:
		return v.textStep(
			"Enter a fine-grained GitHub Personal Access Token (Contents + Metadata, read-only).",
			"Create one at https://github.com/settings/personal-access-tokens/new")
	case stepEmbedProvider:
		return v.selectStep("Choose an embedding provider:", embedProviderOptions, "")
	case stepEmbedKey:
		return v.textStep(fmt.Sprintf("Enter your %s API key.", v.embedProv), providerKeyHint(v.embedProv))
	case stepGenProvider:
		return v.selectStep("Choose a generation (LLM) provider:", genProviderOptions, "")
	case stepGenReuse:
		return v.selectStep(
			fmt.Sprintf("%s is already your embedding provider.", v.genProv),
			reuseKeyOptions, "")
	case stepGenKey:
		return v.textStep(fmt.Sprintf("Enter your %s API key.", v.genProv), providerKeyHint(v.genProv))
	case stepDBPath:
		return v.textStep("Where should the database live?", "Press Enter to accept the default.")
	case stepDone:
		return okStyle.Render("✓ Setup complete!")
	}
	return ""
}

func (v *settingsView) textStep(prompt, hint string) string {
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n")
	b.WriteString(v.input.View())
	if hint != "" {
		b.WriteString("\n\n" + helpStyle.Render(hint))
	}
	return b.String()
}

func (v *settingsView) selectStep(prompt string, opts []string, hint string) string {
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n")
	for i, o := range opts {
		if i == v.cursor {
			b.WriteString(okStyle.Render("› "+o) + "\n")
		} else {
			b.WriteString("  " + o + "\n")
		}
	}
	if hint != "" {
		b.WriteString("\n" + helpStyle.Render(hint))
	}
	return b.String()
}

// --- messages & commands ---------------------------------------------------

// patValidatedMsg / embedValidatedMsg / genValidatedMsg carry the result of an
// async credential check back to Update. settingsSavedMsg carries the result of
// persisting the config (with a ready-to-use *appContext on success).
type patValidatedMsg struct {
	login string
	err   error
}

type embedValidatedMsg struct{ err error }

type genValidatedMsg struct{ err error }

type settingsSavedMsg struct {
	app *appContext
	err error
}

// settingsDoneMsg / settingsCancelledMsg are consumed by the root model (not
// this view) to adopt the new config or return to the tabs.
type settingsDoneMsg struct{ app *appContext }

type settingsCancelledMsg struct{}

// validatePATCmd checks the GitHub token off the UI goroutine, reusing the same
// github.ValidatePAT the init command calls.
func validatePATCmd(pat string) tea.Cmd {
	return func() tea.Msg {
		client := github.NewClient(pat)
		res := github.ValidatePAT(context.Background(), client)
		if !res.Valid {
			return patValidatedMsg{err: fmt.Errorf("GitHub token invalid: %s", res.Error)}
		}
		return patValidatedMsg{login: res.Login}
	}
}

// validateEmbedCmd builds the embedding provider and validates its credentials,
// mirroring selectEmbeddingProvider in commands/init.go.
func validateEmbedCmd(cfg config.ProviderConfig, key string) tea.Cmd {
	return func() tea.Msg {
		p, err := provider.NewEmbeddingProvider(cfg, key)
		if err != nil {
			return embedValidatedMsg{err: fmt.Errorf("create embedding provider: %w", err)}
		}
		if err := p.Validate(context.Background()); err != nil {
			return embedValidatedMsg{err: fmt.Errorf("embedding validation failed: %w", err)}
		}
		return embedValidatedMsg{}
	}
}

func validateGenCmd(cfg config.ProviderConfig, key string) tea.Cmd {
	return func() tea.Msg {
		p, err := provider.NewLLMProvider(cfg, key)
		if err != nil {
			return genValidatedMsg{err: fmt.Errorf("create generation provider: %w", err)}
		}
		if err := p.Validate(context.Background()); err != nil {
			return genValidatedMsg{err: fmt.Errorf("generation validation failed: %w", err)}
		}
		return genValidatedMsg{}
	}
}

// save persists credentials to the keyring and the config file, opens the
// database, and returns a ready *appContext. It captures the collected values so
// the closure is self-contained. Mirrors the save sequence in commands/init.go.
func (v *settingsView) save() tea.Cmd {
	pat := v.githubPAT
	embedCfg, embedKey := v.embedCfg, v.embedKey
	genCfg, genKey := v.genCfg, v.genKey
	dbPath := v.dbPath
	return func() tea.Msg {
		database, err := db.Open(dbPath, embedCfg.Dimensions)
		if err != nil {
			return settingsSavedMsg{err: fmt.Errorf("open database: %w", err)}
		}
		if err := config.SetAPIKeyForProvider("github", pat); err != nil {
			_ = database.Close()
			return settingsSavedMsg{err: fmt.Errorf("save GitHub token: %w", err)}
		}
		if err := config.SetAPIKeyForProvider(embedCfg.Provider, embedKey); err != nil {
			_ = database.Close()
			return settingsSavedMsg{err: fmt.Errorf("save embedding key: %w", err)}
		}
		if err := config.SetAPIKeyForProvider(genCfg.Provider, genKey); err != nil {
			_ = database.Close()
			return settingsSavedMsg{err: fmt.Errorf("save generation key: %w", err)}
		}
		cfg := &config.Config{DBPath: dbPath, Embedding: embedCfg, Generation: genCfg}
		if err := config.SaveConfigFile(cfg); err != nil {
			_ = database.Close()
			return settingsSavedMsg{err: fmt.Errorf("save config: %w", err)}
		}
		return settingsSavedMsg{app: &appContext{cfg: cfg, database: database}}
	}
}

// --- provider defaults (mirror commands/init.go) ---------------------------

func defaultEmbedConfigFor(p string) config.ProviderConfig {
	switch p {
	case "openai":
		return config.ProviderConfig{Provider: "openai", Model: "text-embedding-3-small", Dimensions: 1536}
	case "openrouter":
		return config.ProviderConfig{Provider: "openrouter", Model: "openai/text-embedding-3-small", Dimensions: 1536}
	case "voyageai":
		return config.ProviderConfig{Provider: "voyageai", Model: "voyage-code-3", Dimensions: 1024}
	case "ollama":
		return config.ProviderConfig{Provider: "ollama", Model: "nomic-embed-text", Dimensions: 768, BaseURL: "http://localhost:11434"}
	default: // gemini
		return config.DefaultEmbeddingConfig()
	}
}

func defaultGenConfigFor(p string) config.ProviderConfig {
	switch p {
	case "openai":
		return config.ProviderConfig{Provider: "openai", Model: "gpt-4o", Fallback: "gpt-3.5-turbo"}
	case "openrouter":
		return config.ProviderConfig{Provider: "openrouter", Model: "openai/gpt-4o", Fallback: "openai/gpt-3.5-turbo"}
	case "anthropic":
		return config.ProviderConfig{Provider: "anthropic", Model: "claude-3-5-haiku-20241022", Fallback: "claude-3-5-sonnet-20241022"}
	case "ollama":
		return config.ProviderConfig{Provider: "ollama", Model: "llama3.2", Fallback: "llama2", BaseURL: "http://localhost:11434"}
	default: // gemini
		return config.DefaultGenerationConfig()
	}
}

func providerKeyHint(p string) string {
	switch p {
	case "gemini":
		return "Get a key at https://aistudio.google.com/apikey"
	case "openai":
		return "Get a key at https://platform.openai.com/api-keys"
	case "openrouter":
		return "Get a key at https://openrouter.ai/keys"
	case "voyageai":
		return "Get a key at https://dash.voyageai.com"
	case "anthropic":
		return "Get a key at https://console.anthropic.com"
	}
	return ""
}

func indexOf(opts []string, want string) int {
	for i, o := range opts {
		if o == want {
			return i
		}
	}
	return 0
}
