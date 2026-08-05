package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/ask"
	"github.com/hackastak/repog/internal/format"
)

// askView is the Ask tab: a question box over a scrollable answer pane. Like the
// Search view it is a thin presentation layer over internal/ask — AskQuestion
// does the RAG work and streams tokens back; this view collects the question,
// accumulates the streamed answer, and surfaces errors as UI state (never
// os.Exit, unlike commands/ask.go). The answer streams in token-by-token via a
// channel of tea.Msg, the idiomatic Bubbletea pattern for incremental output.
type askView struct {
	app *appContext

	input    textinput.Model
	spinner  spinner.Model
	viewport viewport.Model

	answer string // accumulated streamed answer for the in-flight/last question
	result ask.AskResult
	lastQ  string
	stream chan tea.Msg // active stream channel; nil when idle
	gen    int          // generation counter; stale stream msgs are discarded

	// streamCtx/cancel drive the in-flight stream. cancel tears down the
	// producer goroutine and aborts its HTTP request when a new question
	// supersedes this one or the app quits (see cancelStream); nil when idle.
	streamCtx context.Context
	cancel    context.CancelFunc

	streaming    bool
	answered     bool // a question has completed at least once this session
	noEmbeddings bool // the corpus has no embeddings yet
	err          error

	lastContent string // cache so we only SetContent on change
}

// askChunkLimit retrieves more context than the CLI default; the answer pane has
// room and richer context tends to improve RAG answers.
const askChunkLimit = 10

func newAskView(app *appContext) *askView {
	ti := textinput.New()
	ti.Placeholder = "Ask a question about your repositories…"
	ti.Prompt = "💬 "
	ti.CharLimit = 512
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = helpStyle

	return &askView{
		app:     app,
		input:   ti,
		spinner: sp,
	}
}

func (v *askView) Init() tea.Cmd {
	v.input.Focus()
	return textinput.Blink
}

// capturingText satisfies textInputView: while the question box is focused the
// root model must not steal plain keys (digits, "q", tab) for global actions.
func (v *askView) capturingText() bool { return v.input.Focused() }

// releaseStream cancels the in-flight stream (if any) and clears its context, so
// the producer goroutine unwinds and its HTTP request is aborted. Calling it when
// the stream has already finished is a harmless no-op reset.
func (v *askView) releaseStream() {
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	v.streamCtx = nil
}

// cancelStream implements streamCanceler; the root model calls it on quit so a
// streaming answer doesn't strand its goroutine when the program exits.
func (v *askView) cancelStream() { v.releaseStream() }

func (v *askView) Update(msg tea.Msg) (view, tea.Cmd) {
	switch msg := msg.(type) {
	case askChunkMsg:
		if msg.gen != v.gen {
			return v, nil // a stale chunk from a superseded question
		}
		v.answer += msg.chunk
		return v, waitForStreamMsg(v.streamCtx, v.stream)

	case askDoneMsg:
		if msg.gen != v.gen {
			return v, nil
		}
		v.streaming = false
		v.answered = true
		v.stream = nil
		v.releaseStream()
		v.err = msg.err
		v.noEmbeddings = msg.noEmbeddings
		if msg.err == nil && !msg.noEmbeddings {
			v.result = msg.result
			// The final answer is authoritative; streamed chunks may differ
			// (e.g. the empty-knowledge-base fallback isn't streamed token-wise).
			v.answer = msg.result.Answer
		}
		return v, nil

	case spinner.TickMsg:
		if !v.streaming {
			return v, nil
		}
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			q := strings.TrimSpace(v.input.Value())
			if v.streaming || q == "" {
				return v, nil
			}
			// Tear down any previous stream before starting a new one so a slow
			// earlier answer can't outlive the question that replaced it.
			v.releaseStream()
			v.gen++
			v.streaming = true
			v.answered = false
			v.err = nil
			v.noEmbeddings = false
			v.answer = ""
			v.result = ask.AskResult{}
			v.lastQ = q
			v.lastContent = ""
			v.viewport.GotoTop()
			ch := make(chan tea.Msg, 64)
			v.stream = ch
			ctx, cancel := context.WithCancel(context.Background())
			v.streamCtx = ctx
			v.cancel = cancel
			// runAskCmd reads the first message itself; the read chain then
			// continues from Update on each askChunkMsg. Issuing waitForStreamMsg
			// here too would add a second concurrent reader on ch — don't.
			return v, tea.Batch(v.spinner.Tick, runAskCmd(v.app, q, v.gen, ctx, ch))
		case "esc":
			// Blur the question box so global tab/quit keys work again, letting
			// the user scroll the answer or leave the tab.
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
		// Browsing mode: keys scroll the answer pane.
		var cmd tea.Cmd
		v.viewport, cmd = v.viewport.Update(msg)
		return v, cmd
	}

	return v, nil
}

func (v *askView) View(width, height int) string {
	var b strings.Builder
	b.WriteString(v.input.View())
	b.WriteString("\n\n")

	switch {
	case v.noEmbeddings:
		b.WriteString(helpStyle.Render("No embeddings yet. Run a sync, then embed, from the Sync/Embed tab."))
	case v.err != nil:
		b.WriteString(errStyle.Render("Ask failed: " + v.err.Error()))
	case v.streaming || v.answered:
		// The question box + blank line take two rows; leave the rest for the
		// answer. A streaming answer follows the bottom; a finished one scrolls.
		v.viewport.Width = width
		v.viewport.Height = max(height-2, 1)
		content := v.renderAnswer()
		if content != v.lastContent {
			atBottom := v.viewport.AtBottom()
			v.viewport.SetContent(content)
			v.lastContent = content
			if v.streaming && atBottom {
				v.viewport.GotoBottom()
			}
		}
		b.WriteString(v.viewport.View())
	default:
		b.WriteString(helpStyle.Render("Type a question and press Enter. Answers cite the repositories they draw from."))
	}

	return b.String()
}

func (v *askView) HelpKeys() string {
	if v.input.Focused() {
		return "enter ask · esc browse"
	}
	return "↑/↓ scroll · / edit question"
}

// renderAnswer formats the question, the (possibly partial) answer, and—once the
// stream completes—the source attributions and token/latency metrics. It reuses
// internal/format so similarity rendering matches the CLI.
func (v *askView) renderAnswer() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Q: ") + v.lastQ + "\n\n")

	if v.streaming {
		b.WriteString(v.spinner.View() + helpStyle.Render(" Thinking…"))
		if v.answer != "" {
			b.WriteString("\n\n")
		}
	}
	b.WriteString(v.answer)

	if !v.streaming && v.answered {
		if len(v.result.Sources) > 0 {
			b.WriteString("\n\n" + helpStyle.Render("Sources:") + "\n")
			for _, s := range v.result.Sources {
				sim := okStyle.Render(format.FormatSimilarity(s.Similarity))
				fmt.Fprintf(&b, "  %s %s %s\n",
					titleStyle.Render(s.RepoFullName),
					helpStyle.Render("("+s.ChunkType+")"),
					sim)
			}
		}
		b.WriteString("\n" + helpStyle.Render(fmt.Sprintf(
			"%d in / %d out tokens · %dms",
			v.result.InputTokens, v.result.OutputTokens, v.result.DurationMs)))
	}

	return b.String()
}

// askChunkMsg delivers one streamed token (or token group) of the answer.
type askChunkMsg struct {
	gen   int
	chunk string
}

// askDoneMsg delivers the final outcome of an async ask to Update.
type askDoneMsg struct {
	gen          int
	result       ask.AskResult
	noEmbeddings bool
	err          error
}

// targetTab routes the streamed answer to the Ask view even if the user has
// switched tabs mid-answer (see routedMsg).
func (askChunkMsg) targetTab() tab { return tabAsk }
func (askDoneMsg) targetTab() tab  { return tabAsk }

// runAskCmd validates preconditions on the UI goroutine, then launches the RAG
// query in a background goroutine that streams chunks onto ch and finishes with
// an askDoneMsg. It reuses the shared providers and the same ask.AskQuestion the
// CLI calls; the only TUI-specific logic is the empty-corpus guard, surfaced as
// an empty-state rather than a hard error. ctx cancels the stream (see
// streamMessages), so quitting mid-answer tears the goroutine and its HTTP
// request down instead of leaking them.
func runAskCmd(app *appContext, question string, gen int, ctx context.Context, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		database := app.db()
		if database == nil {
			return askDoneMsg{gen: gen, err: fmt.Errorf("not configured")}
		}

		// Empty-corpus guard (mirrors commands/ask.go): no embeddings means RAG
		// has nothing to retrieve, so point the user at sync/embed.
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM chunk_embeddings").Scan(&count); err != nil {
			return askDoneMsg{gen: gen, err: err}
		}
		if count == 0 {
			return askDoneMsg{gen: gen, noEmbeddings: true}
		}

		embedProvider, err := app.embedProvider()
		if err != nil {
			return askDoneMsg{gen: gen, err: err}
		}
		llmProvider, err := app.llmProvider()
		if err != nil {
			return askDoneMsg{gen: gen, err: err}
		}

		// Stream off a background goroutine: each token is pushed to ch, then a
		// final askDoneMsg. waitForStreamMsg (re-issued per chunk) drains them.
		return streamMessages(ctx, ch, func(send func(tea.Msg)) {
			result, askErr := ask.AskQuestion(ctx, ask.AskOptions{
				Question:          question,
				Limit:             askChunkLimit,
				DB:                database,
				EmbeddingProvider: embedProvider,
				LLMProvider:       llmProvider,
			}, func(chunk string) {
				send(askChunkMsg{gen: gen, chunk: chunk})
			})
			send(askDoneMsg{gen: gen, result: result, err: askErr})
		})
	}
}
