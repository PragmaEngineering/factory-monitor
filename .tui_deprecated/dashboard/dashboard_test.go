package dashboard

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"controls/execution"
	"controls/internal/control"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestCtrlCQuitsFromEveryDialog(t *testing.T) {
	for _, kind := range []string{"new", "answer", "answer-confirm", "retry", "message", "help", "actions", "agents", "error"} {
		t.Run(kind, func(t *testing.T) {
			m := model{dialog: &dialog{kind: kind}}
			_, command := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			require.NotNil(t, command)
			require.IsType(t, tea.QuitMsg{}, command())
		})
	}
}

func TestMonitorDoesNotReportLiveAfterConnectionFailure(t *testing.T) {
	m := model{connected: true, refreshing: true}
	updated, _ := m.Update(snapshot{err: errors.New("service disconnected")})
	m = updated.(model)
	require.Contains(t, ansi.Strip(m.header()), "DISCONNECTED")
	require.False(t, m.refreshing)
	updated, _ = m.Update(snapshot{})
	m = updated.(model)
	require.Contains(t, ansi.Strip(m.header()), "LIVE")
	require.Empty(t, m.connectionError)
}

func TestMonitorDoesNotOverlapRefreshRequests(t *testing.T) {
	m := model{refreshing: true}
	_, command := m.navigationKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	require.Nil(t, command)
	_, command = m.Update(action{message: "Message sent"})
	require.Nil(t, command)
}

func TestNewExecutionSelectionSurvivesOlderRefresh(t *testing.T) {
	m := model{preferredID: "new-run"}
	updated, _ := m.Update(snapshot{executions: []control.Execution{{ID: "old-run"}}})
	m = updated.(model)
	require.Equal(t, "new-run", m.preferredID)
	updated, _ = m.Update(snapshot{executions: []control.Execution{{ID: "old-run"}, {ID: "new-run"}}})
	m = updated.(model)
	require.Equal(t, "new-run", m.executionID())
	require.Empty(t, m.preferredID)
}

func TestAgentOutputWrapsInsteadOfHidingTheEndOfALine(t *testing.T) {
	m := model{rawLogs: true, tab: 1, outputID: "session-1", output: "This is a long diagnostic ending with THE-REAL-ERROR",
		executions: []control.Execution{{ID: "run-1", Configuration: execution.Configuration{Workflow: execution.Workflow{Steps: []execution.Step{{ID: "build"}}}},
			Steps: []control.Step{{Key: "build", Sessions: []string{"session-1"}}}}}}
	lines := m.detailLines(16, 20)
	require.Contains(t, strings.Join(lines, ""), "THE-REAL-ERROR")
	for _, line := range lines {
		require.LessOrEqual(t, ansi.StringWidth(line), 16)
	}
}

func TestMonitorKeepsSelectedExecutionVisible(t *testing.T) {
	m := model{width: 100, height: 28, follow: true}
	for i := range 20 {
		m.executions = append(m.executions, control.Execution{ID: fmt.Sprintf("run-%02d", i), Brief: fmt.Sprintf("Execution %d", i)})
	}
	m.selectedExecution = 17
	view := ansi.Strip(m.View())
	require.Contains(t, view, "run-17")
	require.NotContains(t, view, "run-00")
	require.LessOrEqual(t, lipgloss.Height(m.View()), m.height)
	require.LessOrEqual(t, lipgloss.Width(m.View()), m.width)
}

func TestWorkflowSelectionShowsStageAndStep(t *testing.T) {
	m := model{width: 110, height: 30, follow: true, focus: 1}
	m.executions = []control.Execution{{ID: "run-1", Workflow: "Go coding", Configuration: execution.Configuration{Workflow: execution.Workflow{
		Steps: []execution.Step{{ID: "spec", Stage: "design"}, {ID: "code", Stage: "build"}, {ID: "review", Stage: "build"}},
	}}}}
	m.move(1)
	require.Equal(t, execution.StepID("code"), m.stepKey())
	view := ansi.Strip(m.View())
	require.Contains(t, view, "DESIGN")
	require.Contains(t, view, "BUILD")
	require.Contains(t, view, "code")
}

func TestCurrentStepFollowsProgressUntilManuallySelected(t *testing.T) {
	m := model{executions: []control.Execution{{ID: "run", CurrentStepID: "visit-2", Configuration: execution.Configuration{Workflow: execution.Workflow{
		Steps: []execution.Step{{ID: "spec"}, {ID: "build"}},
	}}, Steps: []control.Step{{ID: "visit-2", Key: "build"}}}}}
	m.restoreSelection("run", "spec")
	require.Equal(t, execution.StepID("build"), m.stepKey())
	m.stepPinned = true
	m.restoreSelection("run", "spec")
	require.Equal(t, execution.StepID("spec"), m.stepKey())
}

func TestTabCyclesOnlyVisiblePanesAndArrowsKeepFocus(t *testing.T) {
	for _, page := range []monitorPage{homePage, messagesPage, logsPage} {
		m := workflowExample()
		m.page = page
		panes := m.visiblePanes()
		for i, focus := range panes {
			m.focus = focus
			for _, key := range []tea.KeyType{tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown} {
				updated, _ := m.Update(tea.KeyMsg{Type: key})
				require.Equal(t, focus, updated.(model).focus)
			}
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.Equal(t, panes[(i+1)%len(panes)], updated.(model).focus)
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
			require.Equal(t, panes[(i+len(panes)-1)%len(panes)], updated.(model).focus)
		}
	}
}

func TestHorizontalArrowsSelectStepsWithinWorkflow(t *testing.T) {
	m := workflowExample()
	m.focus = 1
	m.selectedStep = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	require.Equal(t, 1, m.selectedStep)
	require.True(t, m.stepPinned)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(model)
	require.Equal(t, 0, m.selectedStep)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, 0, updated.(model).selectedStep)
	require.Equal(t, 1, updated.(model).focus)
}

func TestMenuArrowsStayInsideDialog(t *testing.T) {
	m := workflowExample()
	m.focus = 3
	m.dialog = m.actionsDialog()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	require.Equal(t, 1, m.dialog.selected)
	for _, key := range []tea.KeyType{tea.KeyLeft, tea.KeyRight} {
		updated, _ = m.Update(tea.KeyMsg{Type: key})
		m = updated.(model)
		require.Equal(t, 3, m.focus)
		require.Equal(t, 1, m.dialog.selected)
	}
}

func TestFullscreenPaneNavigationAndRestore(t *testing.T) {
	for _, page := range []monitorPage{homePage, messagesPage, logsPage} {
		m := model{width: 120, height: 36, page: page}
		panes := m.visiblePanes()
		for i, focus := range panes {
			m.focus = focus
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
			zoomed := updated.(model)
			require.True(t, zoomed.fullscreen)
			require.Equal(t, focus, zoomed.focus)
			view := ansi.Strip(zoomed.View())
			for _, line := range strings.Split(view, "\n") {
				require.LessOrEqual(t, ansi.StringWidth(line), m.width)
			}
			require.Contains(t, view, "Restore")
			updated, _ = zoomed.Update(tea.KeyMsg{Type: tea.KeyTab})
			zoomed = updated.(model)
			require.True(t, zoomed.fullscreen)
			require.Equal(t, panes[(i+1)%len(panes)], zoomed.focus)
			updated, _ = zoomed.Update(tea.KeyMsg{Type: tea.KeyEsc})
			require.False(t, updated.(model).fullscreen)
		}
	}
}
