package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
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
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func TestReadableLogsReassembleChunksAndKeepUnknownAndFailedRecords(t *testing.T) {
	records := []state.Activity{
		{Source: "agent/a", Data: []byte(`{"type":"tool_execution_end","toolName":"bash","isError":true,"result":{"content":[{"text":"specific fail`)},
		{Source: "agent/b", Data: []byte("plain output\n")},
		{Source: "agent/a", Data: []byte("ure" + `"}]}}` + "\n" + `{"type":"future_runner_event","data":"still visible"}` + "\n")},
	}
	entries := readableLogs(records)
	require.Len(t, entries, 3)
	require.Equal(t, "plain output", entries[0].body)
	require.True(t, entries[1].failed)
	require.Contains(t, entries[1].body, "specific failure")
	require.Contains(t, entries[2].body, "still visible")
	require.Contains(t, entries[2].kind, "future_runner_event")
	require.Contains(t, entries[1].raw, `"isError":true`)
}

func TestProtocolCompactionRetainsEveryOriginalRecord(t *testing.T) {
	raw := "{\"type\":\"message_start\"}\n{\"type\":\"message_update\",\"delta\":\"abc\"}\n{\"type\":\"message_update\",\"delta\":\"def\"}\n"
	entries := readableLogs([]state.Activity{{Source: "agent/a", Data: []byte(raw)}})
	require.Len(t, entries, 1)
	require.Equal(t, 3, entries[0].records)
	require.Equal(t, strings.TrimSuffix(raw, "\n"), entries[0].raw)
}

func TestLogInspectionPreservesFullResultAndRawMode(t *testing.T) {
	m := model{page: logsPage, focus: 2, activity: []state.Activity{{Source: "agent/a", Data: []byte("long result " + strings.Repeat("x", 1000) + " THE-REAL-ERROR\n")}}}
	require.NotContains(t, strings.Join(m.detailLines(60, 12), ""), "THE-REAL-ERROR")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	require.Contains(t, m.dialog.detail, "THE-REAL-ERROR")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = next.(model)
	require.True(t, m.rawLogs)
	require.Contains(t, strings.Join(strings.Fields(strings.Join(m.detailLines(60, 30), "")), ""), "THE-REAL-ERROR")
}

func TestComposerMultilineKeepsArrowsAndGlobalKeysInInput(t *testing.T) {
	m := messageExample()
	m.focusComposer()
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("first line")})
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyCtrlJ})
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("second line with q 123")})
	require.Equal(t, "first line\nsecond line with q 123", m.composer.Value())
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 4, m.focus)
	require.Equal(t, messagesPage, m.page)
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, "first line\nsecond line with q 123", m.composerDrafts[m.composerDraftKey])
}

func TestExpandedMarkdownAndErrorsFitUnicodeTerminals(t *testing.T) {
	for _, size := range [][2]int{{72, 18}, {80, 24}, {120, 36}} {
		m := model{width: size[0], height: size[1], dialog: &dialog{kind: "inspect", title: "Message", markdown: true, detail: "## Findings\n\n- **Actual issue**\n- " + strings.Repeat("長い説明 ", 80) + "\n\nEND OF MESSAGE", rawDetail: "{\"original\":true}"}}
		require.LessOrEqual(t, lipgloss.Width(m.View()), m.width)
		require.LessOrEqual(t, lipgloss.Height(m.View()), m.height)
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
		m = next.(model)
		require.Contains(t, ansi.Strip(m.View()), "END OF MESSAGE")
		m.dialog = errorDialog("Operation failed", strings.Repeat("長い failure details ", 120)+"END OF ERROR", nil)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
		m = next.(model)
		require.Contains(t, ansi.Strip(m.View()), "END OF ERROR")
		require.LessOrEqual(t, lipgloss.Width(m.View()), m.width)
	}
}

// Optional developer preview: render the real views without starting a service.
func TestMonitorPreview(t *testing.T) {
	destination := os.Getenv("FACTORY_MONITOR_PREVIEW")
	if destination == "" {
		t.Skip("set FACTORY_MONITOR_PREVIEW to write visual-review fixtures")
	}
	require.NoError(t, os.MkdirAll(destination, 0755))
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := inboxExample()
	m.width, m.height = 180, 48
	m.executions[0].Configuration.Workflow.Steps[2].Stage = ""
	m.executions[0].Configuration.Workflow.Steps[5].Stage = ""
	for i := range m.executions[0].Configuration.Workflow.Stages {
		m.executions[0].Configuration.Workflow.Stages[i].Loop.Mode = "concurrent"
	}
	m.executions[0].Steps = append(m.executions[0].Steps, control.Step{ID: "review-visit", Key: "code_review", State: "running", Try: 1, Visit: 2, StartedAt: time.Now().Add(-35 * time.Second)})
	m.executions[0].Requests[0].Kind = "step_input"
	m.executions[0].Requests[0].Prompt = "Confirm whether existing clients must keep their current response format."
	m.conversation = []comms.Message{
		{ID: "feedback", SenderName: "code_review", RecipientNames: []string{"build"}, At: time.Now().Add(-20 * time.Second), Payload: comms.Payload{Kind: "message", Version: 1, Parts: a2a.ContentParts{a2a.TextPart{Text: "F-001: Preserve the existing response format. Please publish the corrected revision for review."}}}},
		{ID: "update", SenderName: "build", RecipientNames: []string{"code_review"}, At: time.Now().Add(-10 * time.Second), Payload: comms.Payload{Kind: "progress", Version: 1, Parts: a2a.ContentParts{a2a.TextPart{Text: "The correction is ready. Running the focused regression checks before publication."}}}},
	}

	m.activity = []state.Activity{
		{Source: "workflow", At: time.Now(), Data: []byte("Independent review started\n")},
		{Source: "agent/reviewer", At: time.Now(), Data: []byte(`{"type":"tool_execution_start","toolName":"bash","args":{"command":"go test -race ./..."}}` + "\n")},
		{Source: "agent/reviewer", At: time.Now(), Data: []byte(`{"type":"tool_execution_end","toolName":"bash","result":{"content":[{"text":"ok example/service 1.42s"}]}}` + "\n")},
		{Source: "agent/reviewer", At: time.Now(), Data: []byte(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Verification complete. No blocking findings."}]}}` + "\n")},
	}
	for _, page := range []monitorPage{homePage, messagesPage, logsPage} {
		m.showPage(page)
		if page == messagesPage {
			m.inboxAgent = "build"
			m.loadComposer()
			m.focusComposer()
			m.composer.SetValue("Please preserve the existing API contract.\nExplain any compatibility concern before proceeding.")
		}
		require.NoError(t, os.WriteFile(filepath.Join(destination, fmt.Sprintf("page-%d.ansi", page+1)), []byte(m.View()), 0644))
	}
	m.dialog = m.newDialog()
	require.NoError(t, os.WriteFile(filepath.Join(destination, "new-execution.ansi"), []byte(m.View()), 0644))
}

func TestPausedLogsKeepSelectedRecordWhileNewOutputArrives(t *testing.T) {
	m := model{page: logsPage, follow: true, activity: []state.Activity{{Source: "agent/a", At: time.Now(), Data: []byte("first record\nsecond record\n")}}}
	m.rebuildLogs()
	m.follow, m.scroll = false, 1
	require.Equal(t, "first record", m.logEntryDialog().detail)
	m.activity = append(m.activity, state.Activity{Source: "agent/a", At: time.Now(), Data: []byte("third record\n")})
	m.rebuildLogs()
	require.Equal(t, "first record", m.logEntryDialog().detail)
	require.Equal(t, 2, m.scroll)
}

func TestRPCFailureCannotBecomeHiddenProtocolChatter(t *testing.T) {
	entry := parseLogRecord("agent/a", `{"type":"response","success":false,"error":"provider unavailable"}`, time.Now())
	require.True(t, entry.failed)
	require.Equal(t, "ERROR", entry.kind)
	require.Contains(t, entry.body, "provider unavailable")
}
