// Package tui provides the Bubble Tea terminal UI for imago.
package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	loop "github.com/benaskins/axon-loop"
	tool "github.com/benaskins/axon-tool"

	"github.com/benaskins/imago/internal/config"
	"github.com/benaskins/imago/internal/session"
)

// phase represents the current application phase.
type phase int

const (
	phaseInterview phase = iota
	phaseDraft
)

// chatEntry is a single item in the conversation view.
type chatEntry struct {
	role      string // "user", "agent", "tool"
	content   string
	collapsed bool // for tool use lines, whether expanded or not
}

// streamEvent is sent over a channel from the LLM goroutine.
type streamEvent struct {
	token   string
	tool    *toolUseMsg
	done    bool
	err     error
	content string // final content on done
}

// streamTickMsg wraps a stream event for the Bubble Tea update loop.
type streamTickMsg struct {
	event streamEvent
	ch    <-chan loop.Event
}

// Model is the top-level Bubble Tea model for imago.
type Model struct {
	// Phase
	phase phase

	// Interview state
	entries   []chatEntry
	streaming string // content being streamed from LLM
	waiting   bool   // true while LLM is generating

	// Components
	input    textarea.Model
	viewport viewport.Model
	width    int
	height   int
	ready    bool

	// LLM
	client   loop.LLMClient
	tools    map[string]tool.ToolDef
	messages []loop.Message // full conversation history

	// Draft state
	sections        []string           // markdown sections of the draft
	rendered        []string           // glamour-rendered sections
	sectionIndex    int                // current section being reviewed
	approved        []bool             // which sections are approved
	draftFinished   bool               // all sections approved
	finalConfirm    bool               // waiting for final confirmation
	finalMarkdown   string             // the complete markdown output
	draftError      string             // error message to display in draft phase
	fullDraft       string             // complete draft text for context
	sectionHistory  [][]loop.Message   // per-section conversation history
	revisionEntries [][]chatEntry      // per-section chat entries for display

	// Session persistence
	session *session.State
}

// New creates a new Model with the given LLM client and tools.
func New(client loop.LLMClient, tools map[string]tool.ToolDef, sess *session.State) Model {
	ta := textarea.New()
	ta.Placeholder = "Type your response..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	m := Model{
		phase:   phaseInterview,
		client:  client,
		tools:   tools,
		input:   ta,
		session: sess,
		messages: []loop.Message{
			{Role: "system", Content: config.SystemPrompt},
		},
	}

	// Restore from session if resuming
	if sess != nil && len(sess.Messages) > 0 {
		m.messages = sess.Messages

		// Rebuild entries from messages for display
		for _, msg := range sess.Messages {
			switch msg.Role {
			case "user":
				m.entries = append(m.entries, chatEntry{role: "user", content: msg.Content})
			case "assistant":
				m.entries = append(m.entries, chatEntry{role: "agent", content: msg.Content})
			}
		}

		if sess.Phase == "draft" && len(sess.Sections) > 0 {
			m.phase = phaseDraft
			m.sections = sess.Sections
			m.approved = sess.Approved
			m.rendered = make([]string, len(sess.Sections))
			for i, s := range sess.Sections {
				m.rendered[i] = renderMarkdown(s, 80)
			}
			// Find first unapproved section
			m.sectionIndex = len(sess.Sections)
			for i, a := range sess.Approved {
				if !a {
					m.sectionIndex = i
					break
				}
			}
			ta.Placeholder = "k/keep to approve, or type revision directive..."
		}

		slog.Info("session resumed", "id", sess.ID, "phase", sess.Phase, "messages", len(sess.Messages))
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.startLLM(config.InterviewModel),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseInterview:
		return m.updateInterview(msg)
	case phaseDraft:
		return m.updateDraft(msg)
	}
	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	switch m.phase {
	case phaseInterview:
		return m.viewInterview()
	case phaseDraft:
		return m.viewDraft()
	}
	return ""
}

// --- Interview phase ---

func (m Model) updateInterview(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.waiting {
				break
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				break
			}
			m.input.Reset()

			// Check for /draft command
			if text == "/draft" {
				return m, func() tea.Msg { return phaseSwitchMsg{} }
			}

			// Add user message
			slog.Info("user message", "phase", "interview", "length", len(text))
			m.entries = append(m.entries, chatEntry{role: "user", content: text})
			m.messages = append(m.messages, loop.Message{Role: "user", Content: text})
			m.waiting = true
			m.streaming = ""
			m.saveSession()
			m.refreshViewport()

			return m, m.startLLM(config.InterviewModel)
		case "tab":
			m.toggleLastToolEntry()
			m.refreshViewport()
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputHeight := 5
		statusHeight := 1

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-inputHeight-statusHeight)
			m.viewport.YPosition = 0
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - inputHeight - statusHeight
		}
		m.input.SetWidth(msg.Width - 2)
		m.refreshViewport()

	case streamTickMsg:
		ev := msg.event
		if ev.err != nil {
			m.entries = append(m.entries, chatEntry{role: "agent", content: fmt.Sprintf("Error: %v", ev.err)})
			m.streaming = ""
			m.waiting = false
			m.refreshViewport()
			return m, nil
		}
		if ev.tool != nil {
			slog.Info("tool use", "tool", ev.tool.name, "args", ev.tool.args)
			label := fmt.Sprintf("\u21b3 %s", ev.tool.name)
			if len(ev.tool.args) > 0 {
				var parts []string
				for k, v := range ev.tool.args {
					parts = append(parts, fmt.Sprintf("%s=%v", k, v))
				}
				label += " " + strings.Join(parts, ", ")
			}
			m.entries = append(m.entries, chatEntry{role: "tool", content: label, collapsed: true})
			m.refreshViewport()
			return m, waitForEvent(msg.ch)
		}
		if ev.done {
			content := ev.content
			if content == "" {
				content = m.streaming
			}
			if content != "" {
				slog.Info("agent response", "phase", "interview", "length", len(content))
				m.entries = append(m.entries, chatEntry{role: "agent", content: content})
				m.messages = append(m.messages, loop.Message{Role: "assistant", Content: content})
				m.saveSession()
			}
			m.streaming = ""
			m.waiting = false
			m.refreshViewport()
			return m, nil
		}
		if ev.token != "" {
			m.streaming += ev.token
			m.refreshViewport()
		}
		return m, waitForEvent(msg.ch)

	case phaseSwitchMsg:
		slog.Info("phase transition", "from", "interview", "to", "draft", "turns", len(m.entries))
		return m.transitionToDraft()
	}

	// Update sub-components
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) saveSession() {
	if m.session == nil {
		m.session = session.New()
	}
	m.session.Messages = m.messages
	if m.phase == phaseDraft {
		m.session.Phase = "draft"
		m.session.Sections = m.sections
		m.session.Approved = m.approved
	}
	if err := m.session.Save(); err != nil {
		slog.Error("failed to save session", "error", err)
	}
}

func (m *Model) refreshViewport() {
	var sb strings.Builder
	w := m.width
	if w <= 0 {
		w = 80
	}

	contentWidth := w - 2
	if contentWidth < 20 {
		contentWidth = 20
	}

	for _, e := range m.entries {
		switch e.role {
		case "user":
			sb.WriteString(userStyle.Render("you") + " ")
			sb.WriteString(wordWrap(e.content, contentWidth-5))
			sb.WriteString("\n\n")
		case "agent":
			sb.WriteString(agentLabelStyle.Render("imago") + " ")
			sb.WriteString(agentStyle.Width(contentWidth-7).Render(e.content))
			sb.WriteString("\n\n")
		case "tool":
			sb.WriteString(toolStyle.Render(e.content))
			sb.WriteString("\n")
		}
	}

	// Show streaming content
	if m.streaming != "" {
		sb.WriteString(agentLabelStyle.Render("imago") + " ")
		sb.WriteString(agentStyle.Width(contentWidth-7).Render(m.streaming))
		sb.WriteString("\u2588") // block cursor
		sb.WriteString("\n")
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func (m *Model) toggleLastToolEntry() {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].role == "tool" {
			m.entries[i].collapsed = !m.entries[i].collapsed
			return
		}
	}
}

func (m Model) currentModel() string {
	switch m.phase {
	case phaseDraft:
		return config.DraftModel
	default:
		return config.InterviewModel
	}
}

func (m Model) viewInterview() string {
	model := modelStyle.Render(m.currentModel())
	status := statusStyle.Render("ctrl+c quit | /draft to start drafting") + "  " + model
	if m.waiting {
		status = statusStyle.Render("thinking...") + "  " + model
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		status,
		m.input.View(),
	)
}

// startLLM launches the LLM via loop.Stream and returns a command that
// reads events from the channel.
func (m Model) startLLM(modelName string) tea.Cmd {
	maxTokens := config.InterviewMaxTokens
	numCtx := config.InterviewNumCtx
	if m.phase == phaseDraft {
		maxTokens = config.DraftMaxTokens
		numCtx = config.DraftNumCtx
	}

	req := &loop.Request{
		Model:     modelName,
		Messages:  m.messages,
		Stream:    true,
		MaxTokens: maxTokens,
		Options:   map[string]any{"num_ctx": numCtx},
	}

	if len(m.tools) > 0 {
		for _, td := range m.tools {
			req.Tools = append(req.Tools, td)
		}
	}

	ch := loop.Stream(context.Background(), m.client, req, m.tools, nil)
	return waitForEvent(ch)
}

// wordWrap wraps text at the given width on word boundaries.
func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var sb strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if len(line) <= width {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
			continue
		}
		words := strings.Fields(line)
		col := 0
		for i, w := range words {
			if i > 0 && col+1+len(w) > width {
				sb.WriteString("\n")
				col = 0
			} else if i > 0 {
				sb.WriteString(" ")
				col++
			}
			sb.WriteString(w)
			col += len(w)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// waitForEvent reads the next loop.Event from the stream channel and
// converts it to a streamTickMsg for Bubble Tea's update loop.
func waitForEvent(ch <-chan loop.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamTickMsg{event: streamEvent{done: true}}
		}
		se := streamEvent{}
		switch {
		case ev.Err != nil:
			se.err = ev.Err
		case ev.ToolUse != nil:
			se.tool = &toolUseMsg{name: ev.ToolUse.Name, args: ev.ToolUse.Args}
		case ev.Done != nil:
			se.done = true
			se.content = ev.Done.Content
		case ev.Token != "":
			se.token = ev.Token
		}
		return streamTickMsg{event: se, ch: ch}
	}
}
