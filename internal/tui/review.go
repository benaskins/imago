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
	m.Waiting = false
	m.draftError = ""

	m.Input.Reset()
	m.Input.Placeholder = "type feedback or /done to finish..."
	m.Input.Focus()

	slog.Info("entering final review", "article_length", len(m.finalMarkdown))
	m.showReview()
	return m, nil
}

func (m Model) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
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

			if text == "/done" {
				slog.Info("final review complete")
				if m.session != nil {
					m.session.MarkComplete(m.sessionDir)
				}
				path, err := writeDraft(m.finalMarkdown)
				if err != nil {
					slog.Error("failed to write draft", "error", err)
					m.draftError = fmt.Sprintf("Failed to save: %v", err)
					m.showReview()
					return m, nil
				}
				slog.Info("draft saved", "path", path)
				fmt.Printf("\nDraft saved to %s\n", path)
				return m, tea.Quit
			}

			// Add to review conversation
			slog.Info("review feedback", "text", text)
			m.reviewHistory = append(m.reviewHistory, loop.Message{
				Role:    loop.RoleUser,
				Content: text,
			})
			m.reviewEntries = append(m.reviewEntries, face.Entry{
				Role:    face.RoleUser,
				Content: text,
			})
			m.Waiting = true
			m.draftError = ""
			m.showReview()
			return m, m.sendReview()
		}

	case tea.WindowSizeMsg:
		m.Chat.HandleResize(msg)
		m.showReview()
		return m, nil

	case reviewMsg:
		if msg.err != nil {
			slog.Error("review error", "error", msg.err)
			m.draftError = msg.err.Error()
			m.Waiting = false
			m.showReview()
			return m, nil
		}

		m.draftError = ""
		m.reviewHistory = append(m.reviewHistory, loop.Message{
			Role:    loop.RoleAssistant,
			Content: msg.content,
		})
		m.reviewEntries = append(m.reviewEntries, face.Entry{
			Role:    face.RoleAgent,
			Content: msg.content,
		})

		// If the response looks like a full article (has headings), update it
		if strings.Contains(msg.content, "\n## ") || strings.HasPrefix(msg.content, "# ") {
			m.finalMarkdown = msg.content
			slog.Info("article updated from review", "length", len(msg.content))
		}

		m.Waiting = false
		m.showReview()
		return m, nil
	}

	cmd := m.Chat.UpdateInput(msg)
	return m, cmd
}

func (m Model) sendReview() tea.Cmd {
	systemPrompt := fmt.Sprintf(config.ReviewPromptTemplate,
		m.interviewTranscript(),
		m.finalMarkdown,
	)

	messages := []loop.Message{
		{Role: loop.RoleSystem, Content: systemPrompt},
	}
	messages = append(messages, m.reviewHistory...)

	req := &loop.Request{
		Model:    m.mcfg.DraftModel,
		Messages: messages,
		Stream:   true,
		Options:  copyMap(m.mcfg.DraftOptions),
	}

	cfg := loop.RunConfig{Client: m.draftLLMClient(), Request: req}
	ch := loop.Stream(context.Background(), cfg)

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

// showReview rebuilds the Chat entries to display the review state.
func (m *Model) showReview() {
	m.Entries = nil

	if m.draftError != "" && !m.Waiting {
		m.Chat.AppendEntry(face.Entry{
			Role:    face.RoleAgent,
			Content: fmt.Sprintf("Error: %s", m.draftError),
		})
		m.RefreshViewport()
		return
	}

	// Show the full article
	m.Chat.AppendEntry(face.Entry{
		Role:    face.RoleAgent,
		Content: fmt.Sprintf("Final review\n\n---\n\n%s\n\n---\n\ntype feedback | /done to finish", m.finalMarkdown),
	})

	// Show review conversation
	for _, e := range m.reviewEntries {
		if e.Role == face.RoleAgent {
			m.Chat.AppendEntry(face.Entry{
				Role:    face.RoleAgent,
				Content: revisionPreview(e.Content),
			})
		} else {
			m.Chat.AppendEntry(e)
		}
	}

	if m.Waiting {
		m.Chat.AppendEntry(face.Entry{Role: face.RoleAction, Content: "reviewing..."})
	}

	m.RefreshViewport()
}

func (m Model) viewReview() string {
	model := m.Styles.Model.Render(m.currentModel())
	status := m.Styles.Status.Render("type feedback | /done to finish | ctrl+c quit") + "  " + model
	if m.Waiting {
		status = m.Styles.Status.Render("thinking...") + "  " + model
	}
	return m.Chat.View(status)
}
