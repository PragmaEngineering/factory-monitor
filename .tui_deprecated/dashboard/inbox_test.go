package dashboard

import (
	"strings"
	"testing"
	"time"

	"controls/internal/control"
	"controls/internal/sessions"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func inboxExample() model {
	now := time.Now()
	m := workflowExample()
	m.page = messagesPage
	m.executions[0].Requests = []control.Request{{ID: "decision", ExecutionID: "preview-run", State: "pending", Prompt: "Review the design", CreatedAt: now.Add(-time.Minute)}}
	m.sessions = []sessions.Session{{ID: "coder", Name: "coder_golang", ExecutionID: "preview-run", State: sessions.Running}, {ID: "reviewer", Name: "code_reviewer_golang", ExecutionID: "preview-run", State: sessions.Running}, {ID: "other", Name: "other-agent", ExecutionID: "unrelated", State: sessions.Running}}
	m.messages = []sessions.Message{{ID: "older", SessionID: "coder", Body: strings.Repeat("Detailed feedback. ", 30) + "END OF MESSAGE", CreatedAt: now.Add(-time.Second), State: sessions.MessageRead}, {ID: "newer", SessionID: "reviewer", Sender: "operator-j", Body: "Check the edge cases", CreatedAt: now, State: sessions.MessageQueued}, {ID: "foreign", SessionID: "other", Body: "Not this execution", CreatedAt: now}}
	return m
}

func TestInboxAggregatesExecutionSessionsAndPrioritizesDecisions(t *testing.T) {
	m := inboxExample()
	items := m.inboxItems()
	require.Len(t, items, 3)
	require.Equal(t, "decision", items[0].id)
	require.Equal(t, "newer", items[1].id)
	require.Equal(t, "operator-j", items[1].sender)
	require.Equal(t, "code_reviewer_golang", items[1].recipient)
	require.Equal(t, "operator", items[2].sender)
	lines := ansi.Strip(strings.Join(m.inboxLines(90, 25), "\n"))
	require.Contains(t, lines, "code_reviewer_golang:operator-j@")
	require.NotContains(t, lines, "Not this execution")
	m.inboxAgent = "coder_golang"
	items = m.inboxItems()
	require.Len(t, items, 1)
	require.Equal(t, "older", items[0].id)
	m.inboxAgent = "@decisions"
	items = m.inboxItems()
	require.Len(t, items, 1)
	require.Equal(t, "decision", items[0].id)
}

func TestInboxSelectionDoesNotSwitchTargetsAfterRefresh(t *testing.T) {
	m := inboxExample()
	m.inboxSelected = "older"
	m.messages = append(m.messages, sessions.Message{ID: "just-in", SessionID: "coder", Body: "New arrival", CreatedAt: time.Now().Add(time.Second)})
	require.Equal(t, "older", m.selectedInboxItem().id)
	require.Contains(t, m.inboxItemDialog().detail, "END OF MESSAGE")
	m.messages = m.messages[1:]
	require.Nil(t, m.selectedInboxItem())
	require.Nil(t, m.inboxItemDialog())
}

func TestAgentSelectionSupportsIndependentConcurrentSessions(t *testing.T) {
	m := inboxExample()
	m.dialog = m.agentSessionsDialog()
	require.Len(t, m.dialog.sessions, 2)
	m.dialog.selected = 1
	updated, _ := m.dialogKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	require.Equal(t, "reviewer", m.sessionID())
	require.Equal(t, 1, m.tab)
	require.Equal(t, sessions.Running, m.activeSession().State)
	m.selectedSessionID = "other"
	require.NotEqual(t, "other", m.sessionID())
}

func TestCopiedLocationsAreShownAsSuccess(t *testing.T) {
	m := model{refreshing: true, dialog: &dialog{kind: "inspect", title: "Review files"}}
	updated, _ := m.Update(action{message: "Copied review file locations"})
	m = updated.(model)
	require.Empty(t, m.dialog.err)
	require.Equal(t, "Copied review file locations", m.dialog.notice)
}
