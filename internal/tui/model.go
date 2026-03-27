// Package tui provides the Bubble Tea terminal UI for imago.
package tui

import (
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	face "github.com/benaskins/axon-face"
	loop "github.com/benaskins/axon-loop"
	tool "github.com/benaskins/axon-tool"

	"github.com/benaskins/imago/internal/config"
)

// phase represents the current application phase.
type phase int

const (
	phaseInterview phase = iota
	phaseDraft
	phaseReview
)

// phaseSwitchMsg triggers a transition from interview to draft.
type phaseSwitchMsg struct{}

// sectionReviseMsg carries the revised section content.
type sectionReviseMsg struct {
	content string
	err     error
}

// Model is the top-level Bubble Tea model for imago.
type Model struct {
	face.Chat

	phase       phase
	client      loop.LLMClient
	draftClient loop.LLMClient // optional: separate client for draft/revision phases
	mcfg        config.ModelConfig
	tools       map[string]tool.ToolDef

	// Draft state
	sections     []string
	sectionIndex int
	approved        []bool
	finalMarkdown   string
	draftError      string
	fullDraft       string
	sectionHistory  [][]loop.Message
	revisionEntries [][]face.Entry

	// Review state (final full-article review)
	reviewHistory []loop.Message
	reviewEntries []face.Entry

	// Prompt overrides
	draftPrompt string // defaults to config.DraftPrompt

	// Session persistence
	session     *face.Session
	sessionDir  string
	sessionKind string // "post" or "weekly"
}

// WithDraftClient sets a separate LLM client for draft/revision phases.
func (m *Model) WithDraftClient(c loop.LLMClient) {
	m.draftClient = c
}

// WithWeeklyMode configures the model for weekly update writing.
func (m *Model) WithWeeklyMode(systemPrompt string) {
	m.sessionKind = "weekly"
	m.draftPrompt = config.WeeklyDraftPrompt
	if len(m.Messages) > 0 && m.Messages[0].Role == loop.RoleSystem {
		m.Messages[0].Content = systemPrompt
	}
}

// draftLLMClient returns the client to use for draft/revision phases.
func (m *Model) draftLLMClient() loop.LLMClient {
	if m.draftClient != nil {
		return m.draftClient
	}
	return m.client
}

// New creates a new Model with the given LLM client and tools.
func New(client loop.LLMClient, mcfg config.ModelConfig, tools map[string]tool.ToolDef, sess *face.Session, sessionDir string) Model {
	chat := face.New("imago")
	chat.Messages = []loop.Message{
		{Role: loop.RoleSystem, Content: config.SystemPrompt()},
	}

	m := Model{
		Chat:        chat,
		phase:       phaseInterview,
		client:      client,
		mcfg:        mcfg,
		draftPrompt: config.DraftPrompt,
		tools:       tools,
		session:     sess,
		sessionDir:  sessionDir,
	}

	// Restore from session if resuming
	if sess != nil && len(sess.Messages) > 0 {
		m.Messages = sess.Messages

		// Rebuild entries from messages for display
		for _, msg := range sess.Messages {
			switch msg.Role {
			case loop.RoleUser:
				m.Chat.AppendEntry(face.Entry{Role: face.RoleUser, Content: msg.Content})
			case loop.RoleAssistant:
				m.Chat.AppendEntry(face.Entry{Role: face.RoleAgent, Content: msg.Content})
			}
		}

		if sess.Phase == "draft" {
			sections, _ := sessionSections(sess)
			approved, _ := sessionApproved(sess)
			if len(sections) > 0 {
				m.phase = phaseDraft
				m.sections = sections
				m.approved = approved
				// Find first unapproved section
				m.sectionIndex = len(sections)
				for i, a := range approved {
					if !a {
						m.sectionIndex = i
						break
					}
				}
				m.Input.Placeholder = "k/keep to approve, or type revision directive..."
			}
		}

		slog.Info("session resumed", "id", sess.ID, "phase", sess.Phase, "messages", len(sess.Messages))
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.Chat.InitCmd(),
		m.startLLM(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseInterview:
		return m.updateInterview(msg)
	case phaseDraft:
		return m.updateDraft(msg)
	case phaseReview:
		return m.updateReview(msg)
	}
	return m, nil
}

func (m Model) View() string {
	if !m.Ready {
		return "Initializing..."
	}

	switch m.phase {
	case phaseInterview:
		return m.viewInterview()
	case phaseDraft:
		return m.viewDraft()
	case phaseReview:
		return m.viewReview()
	}
	return ""
}

// --- Interview phase ---

func (m Model) updateInterview(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Check for /draft command before base handling
		if msg.String() == "enter" && !m.Waiting {
			text := strings.TrimSpace(m.Input.Value())
			if text == "/draft" {
				m.Input.Reset()
				return m, func() tea.Msg { return phaseSwitchMsg{} }
			}
		}

		cmd, handled := m.Chat.HandleKey(msg)
		if handled {
			if cmd != nil {
				return m, cmd
			}
			// enter was handled (user message sent) -- start the stream
			if msg.String() == "enter" && m.Waiting {
				m.saveSession()
				return m, m.startLLM()
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.Chat.HandleResize(msg)
		return m, nil

	case face.StreamTickMsg:
		cmd := m.Chat.HandleStreamTick(msg)
		if cmd == nil {
			// Stream done -- save session
			m.saveSession()
		}
		return m, cmd

	case phaseSwitchMsg:
		slog.Info("phase transition", "from", "interview", "to", "draft", "turns", len(m.Entries))
		return m.transitionToDraft()
	}

	cmd := m.Chat.UpdateInput(msg)
	return m, cmd
}

func (m Model) viewInterview() string {
	model := m.Styles.Model.Render(m.currentModel())
	status := m.Styles.Status.Render("ctrl+c quit | /draft to start drafting") + "  " + model
	if m.Waiting {
		status = m.Styles.Status.Render("thinking...") + "  " + model
	}
	return m.Chat.View(status)
}

func (m Model) currentModel() string {
	switch m.phase {
	case phaseDraft, phaseReview:
		return m.mcfg.DraftModel
	default:
		return m.mcfg.InterviewModel
	}
}

func (m Model) startLLM() tea.Cmd {
	req := &loop.Request{
		Model:   m.mcfg.InterviewModel,
		Options: copyMap(m.mcfg.InterviewOptions),
	}
	return m.Chat.StartStream(m.client, req, m.tools)
}

// --- Session persistence ---

func (m *Model) saveSession() {
	if m.session == nil {
		m.session = face.NewSession()
		if m.session.State == nil {
			m.session.State = make(map[string]any)
		}
		m.session.State["kind"] = m.sessionKind
	}
	m.session.Messages = m.Messages
	if m.phase == phaseDraft {
		m.session.Phase = "draft"
		m.session.State["sections"] = m.sections
		m.session.State["approved"] = m.approved
	}
	if err := m.session.Save(m.sessionDir); err != nil {
		slog.Error("failed to save session", "error", err)
	}
}

// sessionSections extracts sections from session State map.
func sessionSections(sess *face.Session) ([]string, bool) {
	raw, ok := sess.State["sections"]
	if !ok {
		return nil, false
	}
	switch v := raw.(type) {
	case []string:
		return v, true
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			s, _ := item.(string)
			result[i] = s
		}
		return result, true
	}
	return nil, false
}

// sessionApproved extracts approved flags from session State map.
func sessionApproved(sess *face.Session) ([]bool, bool) {
	raw, ok := sess.State["approved"]
	if !ok {
		return nil, false
	}
	switch v := raw.(type) {
	case []bool:
		return v, true
	case []any:
		result := make([]bool, len(v))
		for i, item := range v {
			b, _ := item.(bool)
			result[i] = b
		}
		return result, true
	}
	return nil, false
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
