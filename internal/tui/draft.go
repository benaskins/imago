package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	loop "github.com/benaskins/axon-loop"

	"github.com/benaskins/imago/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// transitionToDraft switches from interview to draft phase.
func (m Model) transitionToDraft() (tea.Model, tea.Cmd) {
	m.phase = phaseDraft
	m.waiting = true
	m.streaming = ""
	m.draftError = ""

	// Reset input for draft controls
	m.input.Reset()
	m.input.Placeholder = "/keep to approve, or type feedback..."
	m.input.Focus()

	slog.Info("draft generation starting", "model", m.mcfg.DraftModel, "provider", m.mcfg.Provider, "messages", len(m.messages))

	// Build the draft request: full transcript + draft prompt
	draftMessages := make([]loop.Message, len(m.messages))
	copy(draftMessages, m.messages)
	draftMessages = append(draftMessages, loop.Message{
		Role:    "user",
		Content: config.DraftPrompt,
	})

	m.messages = draftMessages

	req := &loop.Request{
		Model:     m.mcfg.DraftModel,
		Messages:  draftMessages,
		Stream:    true,
		MaxTokens: m.mcfg.DraftMaxTokens,
		Options:   copyMap(m.mcfg.DraftOptions),
	}

	ch := loop.Stream(context.Background(), m.client, req, nil, nil)
	return m, waitForEvent(ch)
}

func (m Model) updateDraft(msg tea.Msg) (tea.Model, tea.Cmd) {
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

			// Guard: no sections to work with
			if len(m.sections) == 0 {
				slog.Info("retrying draft generation")
				return m.transitionToDraft()
			}

			// Handle section approval
			if text == "/keep" || text == "/k" {
				slog.Info("section approved", "section", m.sectionIndex+1, "total", len(m.sections))
				m.approved[m.sectionIndex] = true
				m.sectionIndex++
				m.saveSession()

				// Check if all sections approved — transition to final review
				if m.sectionIndex >= len(m.sections) {
					return m.transitionToReview()
				}

				m.refreshDraftViewport()
				return m, nil
			}

			// Conversational revision — add to section history
			slog.Info("section feedback", "section", m.sectionIndex+1, "text", text)
			idx := m.sectionIndex
			m.sectionHistory[idx] = append(m.sectionHistory[idx], loop.Message{
				Role:    "user",
				Content: text,
			})
			m.revisionEntries[idx] = append(m.revisionEntries[idx], chatEntry{
				role:    "user",
				content: text,
			})
			m.waiting = true
			m.draftError = ""
			m.refreshDraftViewport()
			return m, m.reviseSection()
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
		m.refreshDraftViewport()

	case streamTickMsg:
		ev := msg.event
		if ev.err != nil {
			slog.Error("draft stream error", "error", ev.err)
			m.draftError = ev.err.Error()
			m.streaming = ""
			m.waiting = false
			m.refreshDraftViewport()
			return m, nil
		}
		if ev.done {
			content := ev.content
			if content == "" {
				content = m.streaming
			}
			slog.Info("draft generation complete", "content_length", len(content))
			if content != "" {
				m.parseSections(content)
				slog.Info("draft parsed", "sections", len(m.sections))
				m.saveSession()
			} else {
				slog.Warn("draft generation produced no content")
				m.draftError = "Draft generation produced no content. Press enter to retry."
			}
			m.streaming = ""
			m.waiting = false
			m.refreshDraftViewport()
			return m, nil
		}
		if ev.token != "" {
			m.streaming += ev.token
			m.refreshDraftViewport()
		}
		return m, waitForEvent(msg.ch)

	case sectionReviseMsg:
		if msg.err != nil {
			slog.Error("section revision error", "error", msg.err)
			m.draftError = msg.err.Error()
			m.waiting = false
			m.refreshDraftViewport()
			return m, nil
		}
		m.draftError = ""

		idx := m.sectionIndex

		// Add agent response to section history
		m.sectionHistory[idx] = append(m.sectionHistory[idx], loop.Message{
			Role:    "assistant",
			Content: msg.content,
		})
		m.revisionEntries[idx] = append(m.revisionEntries[idx], chatEntry{
			role:    "agent",
			content: msg.content,
		})

		// Update the section with the agent's response
		m.sections[idx] = msg.content
		m.rendered[idx] = renderMarkdown(msg.content, m.width)
		m.waiting = false
		m.saveSession()
		m.refreshDraftViewport()
		return m, nil
	}

	// Update sub-components
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) parseSections(content string) {
	m.fullDraft = content
	m.sections = splitSections(content)

	if len(m.sections) == 0 {
		m.sections = []string{content}
	}

	m.approved = make([]bool, len(m.sections))
	m.rendered = make([]string, len(m.sections))
	m.sectionHistory = make([][]loop.Message, len(m.sections))
	m.revisionEntries = make([][]chatEntry, len(m.sections))
	for i, s := range m.sections {
		m.rendered[i] = renderMarkdown(s, m.width)
	}
	m.sectionIndex = 0
}

// interviewTranscript formats the interview messages as a readable transcript
// for inclusion in revision context.
func (m Model) interviewTranscript() string {
	var sb strings.Builder
	for _, msg := range m.messages {
		switch msg.Role {
		case "system":
			continue
		case "user":
			// Skip the draft prompt (last user message)
			if msg.Content == config.DraftPrompt {
				continue
			}
			sb.WriteString("**Author:** ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		case "assistant":
			sb.WriteString("**Interviewer:** ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// reviseSection sends the current section to the LLM with full context
// (interview transcript, full draft, section history).
func (m Model) reviseSection() tea.Cmd {
	idx := m.sectionIndex
	systemPrompt := fmt.Sprintf(config.RevisionPromptTemplate,
		m.interviewTranscript(),
		m.fullDraft,
		m.sections[idx],
	)

	// Build messages: system prompt + conversation history for this section
	messages := []loop.Message{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, m.sectionHistory[idx]...)

	req := &loop.Request{
		Model:    m.mcfg.DraftModel,
		Messages: messages,
		Stream:   true,
		Options:  copyMap(m.mcfg.RevisionOptions),
	}

	ch := loop.Stream(context.Background(), m.client, req, nil, nil)

	// Collect the full response then return as sectionReviseMsg
	return func() tea.Msg {
		var content strings.Builder
		for ev := range ch {
			if ev.Err != nil {
				return sectionReviseMsg{err: ev.Err}
			}
			if ev.Token != "" {
				content.WriteString(ev.Token)
			}
			if ev.Done != nil {
				c := ev.Done.Content
				if c == "" {
					c = content.String()
				}
				return sectionReviseMsg{content: c}
			}
		}
		// Channel closed without Done event
		c := content.String()
		if c == "" {
			return sectionReviseMsg{err: fmt.Errorf("revision produced no content")}
		}
		return sectionReviseMsg{content: c}
	}
}

// assembleDraft joins approved sections into a complete markdown document.
func assembleDraft(sections []string) string {
	return strings.Join(sections, "\n\n")
}

func (m *Model) refreshDraftViewport() {
	var sb strings.Builder
	w := m.width
	if w <= 0 {
		w = 80
	}
	contentWidth := w - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Error state
	if m.draftError != "" && !m.waiting {
		sb.WriteString(headerStyle.Render("Draft error"))
		sb.WriteString("\n\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.draftError))
		sb.WriteString("\n\n")
		sb.WriteString(statusStyle.Render("Press enter to retry."))
		m.viewport.SetContent(sb.String())
		return
	}

	if m.waiting && len(m.sections) == 0 {
		// Still generating the draft
		sb.WriteString(headerStyle.Render("Generating draft..."))
		sb.WriteString("\n\n")
		if m.streaming != "" {
			sb.WriteString(m.streaming)
			sb.WriteString("\u2588\n")
		}
		m.viewport.SetContent(sb.String())
		m.viewport.GotoBottom()
		return
	}

	if len(m.sections) == 0 {
		sb.WriteString(headerStyle.Render("No draft content"))
		sb.WriteString("\n\n")
		sb.WriteString(statusStyle.Render("Press enter to retry draft generation."))
		m.viewport.SetContent(sb.String())
		return
	}

	// Show section-by-section review
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Draft review — section %d of %d", m.sectionIndex+1, len(m.sections))))
	sb.WriteString("\n\n")

	// Show status of all sections
	for i := range m.sections {
		marker := "  "
		if i == m.sectionIndex {
			marker = "> "
		}
		if m.approved[i] {
			sb.WriteString(approvedStyle.Render(fmt.Sprintf("%s[%d] approved", marker, i+1)))
		} else if i == m.sectionIndex {
			sb.WriteString(pendingStyle.Render(fmt.Sprintf("%s[%d] reviewing", marker, i+1)))
		} else {
			sb.WriteString(statusStyle.Render(fmt.Sprintf("%s[%d] pending", marker, i+1)))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Render current section
	if m.sectionIndex < len(m.rendered) {
		bordered := sectionBorderStyle.Width(contentWidth).Render(m.rendered[m.sectionIndex])
		sb.WriteString(bordered)
		sb.WriteString("\n")
	}

	// Show revision conversation for current section
	if m.sectionIndex < len(m.revisionEntries) {
		for _, e := range m.revisionEntries[m.sectionIndex] {
			sb.WriteString("\n")
			switch e.role {
			case "user":
				sb.WriteString(userStyle.Render("you") + " ")
				sb.WriteString(wordWrap(e.content, contentWidth-5))
				sb.WriteString("\n")
			case "agent":
				sb.WriteString(agentLabelStyle.Render("imago") + " ")
				// Show a summary, not the full revised section
				preview := revisionPreview(e.content)
				sb.WriteString(agentStyle.Width(contentWidth - 7).Render(preview))
				sb.WriteString("\n")
			}
		}
	}

	if m.waiting {
		sb.WriteString("\n")
		sb.WriteString(statusStyle.Render("revising..."))
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

// revisionPreview returns a short preview of a revision response.
// Since the agent returns the full revised section, we show a summary
// rather than repeating the entire section in the conversation.
func revisionPreview(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) <= 3 {
		return content
	}
	return lines[0] + "\n" + lines[1] + "\n..."
}

func (m Model) viewDraft() string {
	model := modelStyle.Render(m.currentModel())
	status := statusStyle.Render("/keep to approve | type feedback to revise | ctrl+c quit") + "  " + model
	if m.waiting {
		status = statusStyle.Render("generating...") + "  " + model
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		status,
		m.input.View(),
	)
}

func renderMarkdown(md string, width int) string {
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-8),
	)
	if err != nil {
		return md
	}
	rendered, err := r.Render(md)
	if err != nil {
		return md
	}
	return rendered
}
