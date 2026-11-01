package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"controls/execution"
	"controls/internal/control"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestMainMenuKeepsAllTopLevelOptionsAtEverySupportedWidth(t *testing.T) {
	for _, width := range []int{72, 80, 88, 110, 160, 218} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := model{width: width}
			for focus := 0; focus < 4; focus++ {
				m.focus = focus
				lines := strings.Split(ansi.Strip(m.mainMenu()), "\n")
				require.Len(t, lines, 2)
				for _, line := range lines {
					require.Equal(t, width, ansi.StringWidth(line))
				}
				for _, label := range []string{"New", "Actions", "Context", "Agents", "Inbox", "Panes", "Help", "Quit"} {
					require.Contains(t, lines[1], label)
				}
				require.NotContains(t, strings.ToLower(lines[1]), "review")
				require.NotContains(t, lines[1], "copy")
				require.NotContains(t, lines[1], "open")
				if focus > 0 {
					m.focus = 0
					require.Equal(t, strings.Split(ansi.Strip(m.mainMenu()), "\n")[1], lines[1])
				}
			}
		})
	}
}

func TestContextMenuShortcutIsConsistentAcrossPanes(t *testing.T) {
	for focus := 0; focus < 4; focus++ {
		m := inboxExample()
		m.focus = focus
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		require.Nil(t, cmd)
		require.Equal(t, "contexts", updated.(model).dialog.kind)
	}
}

func TestMonitorSupportsSingleAgentWithoutDecisionsOrStages(t *testing.T) {
	m := model{width: 110, height: 30, executions: []control.Execution{{ID: "one-agent", Configuration: execution.Configuration{Workflow: execution.Workflow{Name: "summarize", Steps: []execution.Step{{ID: "summarize", Run: &execution.RunSpec{Package: "summarizer"}, Routes: map[execution.Outcome]execution.StepID{"finished": execution.Complete}}}}}}}}
	view := ansi.Strip(m.View())
	require.Contains(t, view, "summarizer")
	require.NotContains(t, view, "APPROVAL")
	require.NotContains(t, view, "review files")
	require.NotContains(t, view, "spec.html")
	require.Contains(t, view, "1 Home")
	require.NotContains(t, strings.Join(m.actionsDialog().options, " "), "Answer decision")
}

func TestDecisionPresentationUsesConfiguredAuthority(t *testing.T) {
	require.Equal(t, "DECISION", stepOwner(execution.Step{Decision: &execution.DecisionSpec{}}))
	require.Equal(t, "SCHEDULER DECISION", stepOwner(execution.Step{Decision: &execution.DecisionSpec{Authority: "scheduler"}}))
	m := model{dialog: &dialog{kind: "answer", options: []string{"publish", "archive"}, field: 1, fields: []field{inputField("Reason", "", "")}}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd)
	require.Equal(t, "answer-confirm", updated.(model).dialog.kind)
	require.Empty(t, updated.(model).dialog.err)
}
