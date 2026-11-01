package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"controls/internal/sessions"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type inboxRecipient struct{ key, label, status string }
type deliveryResult struct {
	key, body, recipient string
	message              sessions.Message
	err                  error
}

func (m model) recipients() []inboxRecipient {
	result := []inboxRecipient{{"@operator", "Operator", "Human requests and messages"}}
	if current := m.currentExecution(); current != nil {
		for _, step := range current.Configuration.Workflow.Steps {
			if step.Run == nil {
				continue
			}
			_, status, _ := m.stepPresentation(step, time.Now())
			result = append(result, inboxRecipient{string(step.ID), string(step.ID), step.Run.Package + " · " + status})
		}
	}
	return result
}

func (m model) recipientLabel() string {
	for _, recipient := range m.recipients() {
		if recipient.key == m.inboxAgent {
			return recipient.label
		}
	}
	return "Operator"
}

func (m *model) moveRecipient(direction int) bool {
	recipients := m.recipients()
	selected := 0
	for i, recipient := range recipients {
		if recipient.key == m.inboxAgent {
			selected = i
		}
	}
	next := selected + direction
	if next < 0 || next >= len(recipients) {
		return false
	}
	m.saveComposer()
	m.inboxAgent, m.inboxSelected = recipients[next].key, ""
	m.composerError = ""
	m.loadComposer()
	return true
}

func (m model) recipientLines(width, height int) []string {
	lines := []string{tint("↑↓ select inbox · Enter compose", dim, false), ""}
	recipients := m.recipients()
	selected := 0
	for i, recipient := range recipients {
		if recipient.key == m.inboxAgent {
			selected = i
		}
	}
	visible := max(1, (height-2)/3)
	start := max(0, min(selected-visible/2, len(recipients)-visible))
	for i := start; i < min(len(recipients), start+visible); i++ {
		recipient := recipients[i]
		marker, color := "  ", bright
		if i == selected {
			marker, color = "▸ ", accent
		}
		inbox := m
		inbox.inboxAgent = recipient.key
		lines = append(lines,
			selectedRow(marker+recipient.label, width, i == selected, color),
			tint(ansi.Truncate(fmt.Sprintf("  %d items · %s", len(inbox.inboxItems()), recipient.status), width, "…"), dim, false), "")
	}
	return lines[:min(len(lines), max(0, height))]
}

func (m model) recipientSession() *sessions.Session {
	current := m.currentExecution()
	if current == nil || m.inboxAgent == "@operator" {
		return nil
	}
	var newest *sessions.Session
	for i := range m.sessions {
		session := &m.sessions[i]
		if session.ExecutionID != current.ID || session.State != sessions.Running {
			continue
		}
		matches := false
		for _, visit := range current.Steps {
			if string(visit.Key) == m.inboxAgent && visit.ID == session.StepID {
				matches = true
			}
		}
		if session.StepID == "" {
			for _, step := range current.Configuration.Workflow.Steps {
				if string(step.ID) == m.inboxAgent && step.Run != nil && session.Name == step.Run.Package {
					matches = true
				}
			}
		}
		if matches && (newest == nil || session.StartedAt.After(newest.StartedAt)) {
			newest = session
		}
	}
	return newest
}

func (m *model) saveComposer() {
	if m.composerReady && m.composerDraftKey != "" {
		if m.composerDrafts == nil {
			m.composerDrafts = make(map[string]string)
		}
		m.composerDrafts[m.composerDraftKey] = m.composer.Value()
	}
}

func (m *model) loadComposer() {
	m.saveComposer()
	if !m.composerReady {
		m.composer = textarea.New()
		m.composer.ShowLineNumbers = false
		m.composer.FocusedStyle.Base = lipgloss.NewStyle().Foreground(bright).Background(surface)
		m.composer.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(surface)
		m.composer.FocusedStyle.Text = lipgloss.NewStyle().Foreground(bright).Background(surface)
		m.composer.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(accent).Background(surface)
		m.composer.BlurredStyle = m.composer.FocusedStyle
		m.composer.SetHeight(3)
		m.composer.Prompt = "❯ "
		m.composer.CharLimit = 16384
		m.composerReady = true
	}
	m.composerDraftKey = m.executionID() + "/" + m.inboxAgent
	m.composer.SetValue(m.composerDrafts[m.composerDraftKey])
	m.composer.SetWidth(max(1, m.width-messageIndexWidth(m.width)-10))
	m.composer.Placeholder = "Type a message…"
	m.composer.Blur()
}

func (m *model) focusComposer() tea.Cmd {
	if m.inboxAgent == "" || m.inboxAgent == "@operator" {
		m.notice = "Select an agent in the participant index to compose a message."
		return nil
	}
	if !m.composerReady {
		m.loadComposer()
	}
	m.focus = 4
	return m.composer.Focus()
}

func (m model) composerLines(width, height int) []string {
	if m.inboxAgent == "@operator" || m.inboxAgent == "" {
		return []string{tint("Select an agent to send a message. Answer operator requests from the inbox with Enter.", dim, false)}
	}
	composer := m.composer
	composer.SetWidth(max(1, width))
	composer.SetHeight(max(1, height-1))
	lines := strings.Split(composer.View(), "\n")
	hint := "Enter send · Ctrl-J newline · Esc inbox"
	hintColor := dim
	if m.focus != 4 {
		hint = "m compose · Enter send · Ctrl-J newline"
	}
	if m.recipientSession() == nil {
		hint = "No running session · draft retained"
		hintColor = warn
	}
	if m.sending {
		hint = m.runningGlyph("running") + " Sending…"
		hintColor = warn
	}
	if m.composerError != "" {
		hint = m.composerError
		hintColor = bad
	}
	lines = append(lines, tint(ansi.Truncate(hint, width, "…"), hintColor, false))
	return lines[:min(len(lines), max(0, height))]
}

func (m model) composerKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.composer.SetWidth(max(1, m.width-messageIndexWidth(m.width)-7))
	if m.fullscreen {
		m.composer.SetWidth(max(1, m.width-6))
	}
	switch key.String() {
	case "tab", "shift+tab", "esc":
		m.saveComposer()
		m.composer.Blur()
		if key.String() == "esc" {
			m.focus = 3
		} else if key.String() == "tab" {
			m.cyclePane(1)
		} else {
			m.cyclePane(-1)
		}
		return m, nil
	case "ctrl+j":
		m.composer.InsertString("\n")
		return m, nil
	case "enter":
		return m.sendComposer()
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(key)
	return m, cmd
}

func (m model) sendComposer() (tea.Model, tea.Cmd) {
	if m.sending {
		return m, nil
	}
	body := m.composer.Value()
	if strings.TrimSpace(body) == "" || len(body) > 16384 {
		m.composerError = "Message must contain 1–16384 bytes."
		return m, nil
	}
	session := m.recipientSession()
	if session == nil {
		m.composerError = "This agent has no running session. Your draft is retained."
		return m, nil
	}
	m.saveComposer()
	m.sending, m.composerError = true, ""
	key, id, recipient, client := m.composerDraftKey, session.ID, m.recipientLabel(), m.supervisor
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		message, err := client.Send(ctx, id, body)
		return deliveryResult{key: key, body: body, recipient: recipient, message: message, err: err}
	}
}

func (m model) finishMessage(result deliveryResult) (tea.Model, tea.Cmd) {
	m.sending = false
	m.saveComposer()
	if result.err != nil {
		m.composerError = "Message not sent; draft retained."
		m.dialog = errorDialog("Message not sent", result.err.Error(), nil)
		return m, nil
	}
	if m.composerDrafts[result.key] == result.body {
		delete(m.composerDrafts, result.key)
	}
	if m.composerDraftKey == result.key && m.composer.Value() == result.body {
		m.composer.SetValue("")
	}
	m.notice = "Queued to " + result.recipient + "; delivery is not proof of execution."
	return m, nil
}

func messageIndexWidth(width int) int { return max(22, min(36, width/4)) }

func (m model) messagesView(width, height int) string {
	trafficHeight := min(7, max(5, height/4))
	traffic := m.panel("◈ MESSAGE TRAFFIC", m.trafficLines(width-4, trafficHeight-3), width, trafficHeight, false)
	remaining := height - trafficHeight
	leftWidth := messageIndexWidth(width)
	rightWidth := width - leftWidth - 1
	composerHeight := min(8, max(4, remaining/3))
	index := m.panel("INBOX INDEX", m.recipientLines(leftWidth-4, remaining-3), leftWidth, remaining, m.focus == 1)
	inbox := m.panel(m.paneTitle(3), m.inboxLines(rightWidth-4, remaining-composerHeight-3), rightWidth, remaining-composerHeight, m.focus == 3)
	composer := m.panel(m.paneTitle(4), m.composerLines(rightWidth-4, composerHeight-3), rightWidth, composerHeight, m.focus == 4)
	return lipgloss.JoinVertical(lipgloss.Left, traffic, lipgloss.JoinHorizontal(lipgloss.Top, index, " ", lipgloss.JoinVertical(lipgloss.Left, inbox, composer)))
}
