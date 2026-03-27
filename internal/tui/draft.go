package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	face "github.com/benaskins/axon-face"
	loop "github.com/benaskins/axon-loop"

	"github.com/benaskins/imago/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// transitionToDraft switches from interview to draft phase.
func (m Model) transitionToDraft() (tea.Model, tea.Cmd) {
	m.phase = phaseDraft
	m.Waiting = true
	m.Streaming = ""
	m.draftError = ""

	// Reset input for draft controls
	m.Input.Reset()
	m.Input.Placeholder = "/keep to approve, or type feedback..."
	m.Input.Focus()

	slog.Info("draft generation starting", "model", m.mcfg.DraftModel, "provider", m.mcfg.Provider, "messages", len(m.Messages))

	// Build the draft request: full transcript + draft prompt
	draftMessages := make([]loop.Message, len(m.Messages))
	copy(draftMessages, m.Messages)
	draftMessages = append(draftMessages, loop.Message{
		Role:    loop.RoleUser,
		Content: m.draftPrompt,
	})

	m.Messages = draftMessages

	req := &loop.Request{
		Model:     m.mcfg.DraftModel,
		Messages:  draftMessages,
		Stream:    true,
		MaxTokens: m.mcfg.DraftMaxTokens,
		Options:   copyMap(m.mcfg.DraftOptions),
	}

	cfg := loop.RunConfig{Client: m.draftLLMClient(), Request: req}
	ch := loop.Stream(context.Background(), cfg)
	return m, face.WaitForEvent(ch)
}

func (m Model) updateDraft(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.Waiting {
				return m, nil
			}
			text := strings.TrimSpace(m.Input.Value())
			if text == "" {
				return m, nil
			}
			m.Input.Reset()

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

				if m.sectionIndex >= len(m.sections) {
					return m.transitionToReview()
				}

				m.showCurrentSection()
				return m, nil
			}

			// Conversational revision
			slog.Info("section feedback", "section", m.sectionIndex+1, "text", text)
			idx := m.sectionIndex
			m.sectionHistory[idx] = append(m.sectionHistory[idx], loop.Message{
				Role:    loop.RoleUser,
				Content: text,
			})
			m.revisionEntries[idx] = append(m.revisionEntries[idx], face.Entry{
				Role:    face.RoleUser,
				Content: text,
			})
			m.Waiting = true
			m.draftError = ""
			m.showCurrentSection()
			return m, m.reviseSection()
		}

	case tea.WindowSizeMsg:
		m.Chat.HandleResize(msg)
		m.showCurrentSection()
		return m, nil

	case face.StreamTickMsg:
		ev := msg.Event
		if ev.Err != nil {
			slog.Error("draft stream error", "error", ev.Err)
			m.draftError = ev.Err.Error()
			m.Streaming = ""
			m.Waiting = false
			m.showCurrentSection()
			return m, nil
		}
		if ev.Done {
			content := ev.Content
			if content == "" {
				content = m.Streaming
			}
			slog.Info("draft generation complete", "content_length", len(content))
			if content != "" {
				m.parseSections(content)
				slog.Info("draft parsed", "sections", len(m.sections))
				m.saveSession()
				m.showCurrentSection()
			} else {
				slog.Warn("draft generation produced no content")
				m.draftError = "Draft generation produced no content. Press enter to retry."
			}
			m.Streaming = ""
			m.Waiting = false
			m.RefreshViewport()
			return m, nil
		}
		if ev.Token != "" {
			m.Streaming += ev.Token
			m.RefreshViewport()
		}
		return m, face.WaitForEvent(msg.Ch)

	case sectionReviseMsg:
		if msg.err != nil {
			slog.Error("section revision error", "error", msg.err)
			m.draftError = msg.err.Error()
			m.Waiting = false
			m.showCurrentSection()
			return m, nil
		}
		m.draftError = ""

		idx := m.sectionIndex

		// Add agent response to section history
		m.sectionHistory[idx] = append(m.sectionHistory[idx], loop.Message{
			Role:    loop.RoleAssistant,
			Content: msg.content,
		})
		m.revisionEntries[idx] = append(m.revisionEntries[idx], face.Entry{
			Role:    face.RoleAgent,
			Content: msg.content,
		})

		// Update the section with the agent's response
		m.sections[idx] = msg.content
		m.Waiting = false
		m.saveSession()
		m.showCurrentSection()
		return m, nil
	}

	cmd := m.Chat.UpdateInput(msg)
	return m, cmd
}

func (m *Model) parseSections(content string) {
	m.fullDraft = content
	m.sections = splitSections(content)

	if len(m.sections) == 0 {
		m.sections = []string{content}
	}

	m.approved = make([]bool, len(m.sections))
	m.sectionHistory = make([][]loop.Message, len(m.sections))
	m.revisionEntries = make([][]face.Entry, len(m.sections))
	m.sectionIndex = 0
}

// showCurrentSection rebuilds the Chat entries to display the current draft state.
func (m *Model) showCurrentSection() {
	m.Entries = nil

	if m.draftError != "" && !m.Waiting {
		m.Chat.AppendEntry(face.Entry{
			Role:    face.RoleAgent,
			Content: fmt.Sprintf("Error: %s\n\nPress enter to retry.", m.draftError),
		})
		m.RefreshViewport()
		return
	}

	if len(m.sections) == 0 {
		return
	}

	// Section overview
	var overview strings.Builder
	for i := range m.sections {
		marker := "[ ]"
		if m.approved[i] {
			marker = "[v]"
		}
		if i == m.sectionIndex {
			marker = "[>]"
		}
		label := strings.SplitN(m.sections[i], "\n", 2)[0]
		overview.WriteString(fmt.Sprintf("%s %d. %s\n", marker, i+1, label))
	}

	// Current section content
	sectionContent := ""
	if m.sectionIndex < len(m.sections) {
		sectionContent = m.sections[m.sectionIndex]
	}

	m.Chat.AppendEntry(face.Entry{
		Role: face.RoleAgent,
		Content: fmt.Sprintf("Draft review, section %d of %d\n\n%s\n---\n\n%s\n\n/keep to approve, or type revision feedback",
			m.sectionIndex+1, len(m.sections), overview.String(), sectionContent),
	})

	// Show revision conversation for current section
	if m.sectionIndex < len(m.revisionEntries) {
		for _, e := range m.revisionEntries[m.sectionIndex] {
			if e.Role == face.RoleAgent {
				// Show preview, not full revised section
				m.Chat.AppendEntry(face.Entry{
					Role:    face.RoleAgent,
					Content: revisionPreview(e.Content),
				})
			} else {
				m.Chat.AppendEntry(e)
			}
		}
	}

	if m.Waiting {
		m.Chat.AppendEntry(face.Entry{Role: face.RoleAction, Content: "revising..."})
	}

	m.RefreshViewport()
}

// interviewTranscript formats the interview messages as a readable transcript
// for inclusion in revision context.
func (m Model) interviewTranscript() string {
	var sb strings.Builder
	for _, msg := range m.Messages {
		switch msg.Role {
		case loop.RoleSystem:
			continue
		case loop.RoleUser:
			// Skip the draft prompt (last user message)
			if msg.Content == m.draftPrompt {
				continue
			}
			sb.WriteString("**Author:** ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		case loop.RoleAssistant:
			sb.WriteString("**Interviewer:** ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// reviseSection sends the current section to the LLM with full context.
func (m Model) reviseSection() tea.Cmd {
	idx := m.sectionIndex
	systemPrompt := fmt.Sprintf(config.RevisionPromptTemplate,
		m.interviewTranscript(),
		m.fullDraft,
		m.sections[idx],
	)

	messages := []loop.Message{
		{Role: loop.RoleSystem, Content: systemPrompt},
	}
	messages = append(messages, m.sectionHistory[idx]...)

	req := &loop.Request{
		Model:    m.mcfg.DraftModel,
		Messages: messages,
		Stream:   true,
		Options:  copyMap(m.mcfg.RevisionOptions),
	}

	cfg := loop.RunConfig{Client: m.draftLLMClient(), Request: req}
	ch := loop.Stream(context.Background(), cfg)

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

// revisionPreview returns a short preview of a revision response.
func revisionPreview(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) <= 3 {
		return content
	}
	return lines[0] + "\n" + lines[1] + "\n..."
}

func (m Model) viewDraft() string {
	model := m.Styles.Model.Render(m.currentModel())
	status := m.Styles.Status.Render("/keep to approve | type feedback to revise | ctrl+c quit") + "  " + model
	if m.Waiting {
		status = m.Styles.Status.Render("generating...") + "  " + model
	}
	return m.Chat.View(status)
}

