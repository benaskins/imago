package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	loop "github.com/benaskins/axon-loop"

	"github.com/benaskins/imago/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// reviewMsg carries the agent's response during final review.
type reviewMsg struct {
	content string
	err     error
}

// transitionToReview switches from section review to final article review.
func (m Model) transitionToReview() (tea.Model, tea.Cmd) {
	m.phase = phaseReview
	m.finalMarkdown = assembleDraft(m.sections)
	m.reviewHistory = nil
	m.reviewEntries = nil
	m.waiting = false
	m.draftError = ""

	m.input.Reset()
	m.input.Placeholder = "type feedback or /done to finish..."
	m.input.Focus()

	slog.Info("entering final review", "article_length", len(m.finalMarkdown))
	m.refreshReviewViewport()
	return m, nil
}

func (m Model) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
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

			if text == "/done" {
				slog.Info("final review complete")
				if m.session != nil {
					m.session.MarkComplete()
				}
				path, err := writeDraft(m.finalMarkdown)
				if err != nil {
					slog.Error("failed to write draft", "error", err)
					m.draftError = fmt.Sprintf("Failed to save: %v", err)
					m.refreshReviewViewport()
					return m, nil
				}
				slog.Info("draft saved", "path", path)
				// Print path after alt-screen exits so user can see it
				fmt.Printf("\nDraft saved to %s\n", path)
				return m, tea.Quit
			}

			// Add to review conversation
			slog.Info("review feedback", "text", text)
			m.reviewHistory = append(m.reviewHistory, loop.Message{
				Role:    "user",
				Content: text,
			})
			m.reviewEntries = append(m.reviewEntries, chatEntry{
				role:    "user",
				content: text,
			})
			m.waiting = true
			m.draftError = ""
			m.refreshReviewViewport()
			return m, m.sendReview()
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
		m.refreshReviewViewport()

	case reviewMsg:
		if msg.err != nil {
			slog.Error("review error", "error", msg.err)
			m.draftError = msg.err.Error()
			m.waiting = false
			m.refreshReviewViewport()
			return m, nil
		}

		m.draftError = ""
		m.reviewHistory = append(m.reviewHistory, loop.Message{
			Role:    "assistant",
			Content: msg.content,
		})
		m.reviewEntries = append(m.reviewEntries, chatEntry{
			role:    "agent",
			content: msg.content,
		})

		// If the response looks like a full article (has headings), update the article
		if strings.Contains(msg.content, "\n## ") || strings.HasPrefix(msg.content, "# ") {
			m.finalMarkdown = msg.content
			slog.Info("article updated from review", "length", len(msg.content))
		}

		m.waiting = false
		m.refreshReviewViewport()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) sendReview() tea.Cmd {
	systemPrompt := fmt.Sprintf(config.ReviewPromptTemplate,
		m.interviewTranscript(),
		m.finalMarkdown,
	)

	messages := []loop.Message{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, m.reviewHistory...)

	req := &loop.Request{
		Model:    m.mcfg.DraftModel,
		Messages: messages,
		Stream:   true,
		Options:  copyMap(m.mcfg.DraftOptions),
	}

	ch := loop.Stream(context.Background(), m.draftLLMClient(), req, nil, nil)

	return func() tea.Msg {
		var content strings.Builder
		for ev := range ch {
			if ev.Err != nil {
				return reviewMsg{err: ev.Err}
			}
			if ev.Token != "" {
				content.WriteString(ev.Token)
			}
			if ev.Done != nil {
				c := ev.Done.Content
				if c == "" {
					c = content.String()
				}
				return reviewMsg{content: c}
			}
		}
		c := content.String()
		if c == "" {
			return reviewMsg{err: fmt.Errorf("review produced no content")}
		}
		return reviewMsg{content: c}
	}
}

func (m *Model) refreshReviewViewport() {
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
		sb.WriteString(headerStyle.Render("Review error"))
		sb.WriteString("\n\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.draftError))
		sb.WriteString("\n")
		m.viewport.SetContent(sb.String())
		return
	}

	sb.WriteString(headerStyle.Render("Final review"))
	sb.WriteString("\n\n")

	// Render the full article
	rendered := renderMarkdown(m.finalMarkdown, w)
	bordered := sectionBorderStyle.Width(contentWidth).Render(rendered)
	sb.WriteString(bordered)
	sb.WriteString("\n")

	// Show review conversation
	for _, e := range m.reviewEntries {
		sb.WriteString("\n")
		switch e.role {
		case "user":
			sb.WriteString(userStyle.Render("you") + " ")
			sb.WriteString(wordWrap(e.content, contentWidth-5))
			sb.WriteString("\n")
		case "agent":
			sb.WriteString(agentLabelStyle.Render("imago") + " ")
			preview := revisionPreview(e.content)
			sb.WriteString(agentStyle.Width(contentWidth - 7).Render(preview))
			sb.WriteString("\n")
		}
	}

	if m.waiting {
		sb.WriteString("\n")
		sb.WriteString(statusStyle.Render("reviewing..."))
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func (m Model) viewReview() string {
	model := modelStyle.Render(m.currentModel())
	status := statusStyle.Render("type feedback | /done to finish | ctrl+c quit") + "  " + model
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
