package tui

import (
	"context"
	"fmt"
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

	// Reset input for draft controls
	m.input.Reset()
	m.input.Placeholder = "k/keep to approve, or type revision directive..."
	m.input.Focus()

	// Build the draft request: full transcript + draft prompt
	draftMessages := make([]loop.Message, len(m.messages))
	copy(draftMessages, m.messages)
	draftMessages = append(draftMessages, loop.Message{
		Role:    "user",
		Content: config.DraftPrompt,
	})

	m.messages = draftMessages

	ch := make(chan streamEvent, 64)
	go func() {
		defer close(ch)
		ctx := context.Background()

		req := &loop.Request{
			Model:    config.DraftModel,
			Messages: draftMessages,
			Stream:   true,
		}

		var full strings.Builder
		cb := loop.Callbacks{
			OnToken: func(token string) {
				full.WriteString(token)
				ch <- streamEvent{token: token}
			},
		}

		_, err := loop.Run(ctx, m.client, req, nil, nil, cb)
		if err != nil {
			ch <- streamEvent{err: err}
			return
		}

		ch <- streamEvent{done: true, content: full.String()}
	}()

	return m, waitForStream(ch)
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

			if m.finalConfirm {
				if text == "y" || text == "yes" {
					// Print final markdown to stdout and quit
					fmt.Print(m.finalMarkdown)
					return m, tea.Quit
				}
				// Not confirmed — go back to first unapproved section
				m.finalConfirm = false
				m.refreshDraftViewport()
				return m, nil
			}

			// Handle section approval or revision
			if text == "k" || text == "keep" {
				m.approved[m.sectionIndex] = true
				m.sectionIndex++

				// Check if all sections approved
				if m.sectionIndex >= len(m.sections) {
					m.finalConfirm = true
					m.finalMarkdown = strings.Join(m.sections, "\n\n---\n\n")
					m.refreshDraftViewport()
					return m, nil
				}

				m.refreshDraftViewport()
				return m, nil
			}

			// Revision directive — send to LLM
			m.waiting = true
			return m, m.reviseSection(text)
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
			m.entries = append(m.entries, chatEntry{role: "agent", content: fmt.Sprintf("Error: %v", ev.err)})
			m.streaming = ""
			m.waiting = false
			m.refreshDraftViewport()
			return m, nil
		}
		if ev.done {
			if ev.content != "" {
				m.parseSections(ev.content)
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
		return m, waitForStream(msg.ch)

	case sectionReviseMsg:
		m.sections[m.sectionIndex] = msg.content
		m.rendered[m.sectionIndex] = renderMarkdown(msg.content, m.width)
		m.waiting = false
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
	// Split on "---" separator
	parts := strings.Split(content, "\n---\n")
	m.sections = make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			m.sections = append(m.sections, trimmed)
		}
	}

	if len(m.sections) == 0 {
		m.sections = []string{content}
	}

	m.approved = make([]bool, len(m.sections))
	m.rendered = make([]string, len(m.sections))
	for i, s := range m.sections {
		m.rendered[i] = renderMarkdown(s, m.width)
	}
	m.sectionIndex = 0
}

func (m *Model) refreshDraftViewport() {
	var sb strings.Builder
	w := m.width
	if w <= 0 {
		w = 80
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

	if m.finalConfirm {
		sb.WriteString(headerStyle.Render("Draft complete"))
		sb.WriteString("\n\n")

		rendered := renderMarkdown(m.finalMarkdown, w)
		sb.WriteString(rendered)
		sb.WriteString("\n\n")
		sb.WriteString(approvedStyle.Render("All sections approved. Publish? (y/n)"))

		m.viewport.SetContent(sb.String())
		m.viewport.GotoBottom()
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
		bordered := sectionBorderStyle.Width(w - 4).Render(m.rendered[m.sectionIndex])
		sb.WriteString(bordered)
		sb.WriteString("\n")
	}

	if m.waiting {
		sb.WriteString("\n")
		sb.WriteString(statusStyle.Render("revising..."))
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func (m Model) viewDraft() string {
	status := statusStyle.Render("k/keep to approve | type directive to revise | ctrl+c quit")
	if m.waiting {
		status = statusStyle.Render("generating...")
	}
	if m.finalConfirm {
		status = statusStyle.Render("y to publish | n to go back")
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		status,
		m.input.View(),
	)
}

func (m Model) reviseSection(directive string) tea.Cmd {
	ch := make(chan streamEvent, 64)

	go func() {
		defer close(ch)
		ctx := context.Background()

		revisePrompt := fmt.Sprintf(
			"Here is the current section:\n\n%s\n\nRevision directive: %s\n\nRewrite the section applying the directive. Output only the revised section markdown, nothing else.",
			m.sections[m.sectionIndex],
			directive,
		)

		messages := []loop.Message{
			{Role: "system", Content: "You are a skilled editor revising a blog post section. Apply the directive precisely."},
			{Role: "user", Content: revisePrompt},
		}

		req := &loop.Request{
			Model:    config.DraftModel,
			Messages: messages,
			Stream:   true,
		}

		var full strings.Builder
		cb := loop.Callbacks{
			OnToken: func(token string) {
				full.WriteString(token)
				ch <- streamEvent{token: token}
			},
		}

		_, err := loop.Run(ctx, m.client, req, nil, nil, cb)
		if err != nil {
			ch <- streamEvent{err: err}
			return
		}

		ch <- streamEvent{done: true, content: full.String()}
	}()

	// For revision, we handle the stream differently —
	// collect everything then update the section
	return func() tea.Msg {
		var content strings.Builder
		for ev := range ch {
			if ev.err != nil {
				return errMsg{err: ev.err}
			}
			if ev.token != "" {
				content.WriteString(ev.token)
			}
			if ev.done {
				c := ev.content
				if c == "" {
					c = content.String()
				}
				return sectionReviseMsg{content: c}
			}
		}
		return sectionReviseMsg{content: content.String()}
	}
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
