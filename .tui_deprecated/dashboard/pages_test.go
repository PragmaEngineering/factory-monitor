package dashboard

import (
	"strings"
	"testing"
	"time"

	"controls/internal/comms"
	"controls/internal/control"
	"controls/internal/state"

	"github.com/a2aproject/a2a-go/a2a"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestHomeMessagesAndLogsHaveDistinctContent(t *testing.T) {
	m := inboxExample()
	m.width, m.height = 180, 45
	m.activity = []state.Activity{{Source: "agent/test", Data: []byte("raw log evidence")}}
	m.showPage(homePage)
	view := ansi.Strip(m.View())
	require.Contains(t, view, "EXECUTIONS")
	require.Contains(t, view, "WORKFLOW")
	require.Contains(t, view, "OPERATOR INBOX")
	require.NotContains(t, view, "raw log evidence")
	require.NotContains(t, view, "Check the edge cases", "operator's outgoing peer messages do not belong in the Home inbox")
	require.Contains(t, view, "Review the design")

	m.inboxAgent = "old-filter"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(model)
	require.Equal(t, messagesPage, m.page)
	require.Equal(t, "@operator", m.inboxAgent, "Messages opens the operator inbox")
	view = ansi.Strip(m.View())
	require.Contains(t, view, "MESSAGE TRAFFIC")
	require.Contains(t, view, "INBOX INDEX")
	require.Contains(t, view, "INBOX · Operator")
	require.NotContains(t, view, "raw log evidence")
	m.showPage(logsPage)
	view = ansi.Strip(m.View())
	require.Contains(t, view, "raw log evidence")
	require.NotContains(t, view, "INBOX INDEX")
	require.NotContains(t, view, "EXECUTIONS")
	require.NotContains(t, view, "WORKFLOW")
	require.Equal(t, "preview-run", m.executionID())
}

func TestConversationAudienceAndFilter(t *testing.T) {
	m := inboxExample()
	m.messages, m.executions[0].Requests = nil, nil
	now := time.Now()
	m.conversation = []comms.Message{
		{ID: "peer", SenderName: "build", RecipientNames: []string{"code_review"}, Payload: comms.Payload{Kind: "message", Version: 1, Parts: a2a.ContentParts{a2a.TextPart{Text: "Check this revision"}}}, At: now},
		{ID: "human", SenderName: "build", RecipientNames: []string{"human", "controller"}, Payload: comms.Payload{Kind: "message", Version: 1, Parts: a2a.ContentParts{a2a.TextPart{Text: "I need your input"}}}, At: now},
	}
	m.showPage(homePage)
	require.Len(t, m.inboxItems(), 1)
	require.Equal(t, "human", m.inboxItems()[0].id)
	require.Contains(t, ansi.Strip(strings.Join(m.inboxLines(80, 20), "\n")), "I need your input")
	m.showPage(messagesPage)
	require.Len(t, m.inboxItems(), 1, "operator index shows only operator messages")
	m.inboxAgent = "code_review"
	require.Len(t, m.inboxItems(), 1, "agent inbox shows addressed messages")
	require.Equal(t, "peer", m.inboxItems()[0].id)
}

func TestCommunicationMapUsesActualDirectedTraffic(t *testing.T) {
	m := workflowExample()
	m.page = messagesPage
	now := time.Now()
	m.conversation = []comms.Message{
		{SenderName: "build", RecipientNames: []string{"code_review", "controller"}, At: now},
		{SenderName: "build", RecipientNames: []string{"code_review"}, At: now},
		{SenderName: "code_review", RecipientNames: []string{"build"}, At: now.Add(time.Second)},
	}
	edges := m.communicationEdges()
	require.Len(t, edges, 3)
	require.Equal(t, "code_review", edges[0].from)
	require.Equal(t, "build", edges[0].to)
	text := ansi.Strip(strings.Join(m.trafficLines(70, 10), "\n"))
	require.Contains(t, text, "2 ●▶ [code_review]")
	require.Contains(t, text, "1 ●▶ [build]")
	m.conversation = nil
	require.Empty(t, m.communicationEdges(), "workflow routes are not evidence of messages")
}

func TestBothPagesAndFullscreenFitSupportedTerminals(t *testing.T) {
	for _, size := range [][2]int{{72, 18}, {80, 24}, {110, 30}, {160, 45}, {218, 70}} {
		for _, page := range []monitorPage{homePage, messagesPage, logsPage} {
			m := inboxExample()
			m.width, m.height, m.page = size[0], size[1], page
			for _, pane := range m.visiblePanes() {
				m.focus = pane
				for _, expanded := range []bool{false, true} {
					m.fullscreen = expanded
					view := m.View()
					require.LessOrEqual(t, lipgloss.Width(view), m.width)
					require.LessOrEqual(t, lipgloss.Height(view), m.height)
				}
			}
		}
	}
}

func TestLogsRefreshKeepsAnAgentSelectedAtHumanDecision(t *testing.T) {
	m := workflowExample()
	m.executions[0].CurrentStepID = "approval"
	m.executions[0].Steps = append(m.executions[0].Steps, control.Step{ID: "approval", Key: "acceptance", State: "waiting_input"})
	m.showPage(logsPage)
	m.restoreSelection(m.executionID(), "build")
	require.Equal(t, "code_review", string(m.stepKey()))
}
