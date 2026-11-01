package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"controls/execution"
	"controls/internal/control"
	"controls/internal/state"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func workflowExample() model {
	now := time.Now()
	names := []execution.StepID{"specification", "architecture_review", "design_approval", "build", "code_review", "acceptance"}
	owners := []string{"architect", "architecture_reviewer", "", "coder_golang", "code_reviewer_golang", ""}
	steps := make([]execution.Step, len(names))
	for i, name := range names {
		stage := "design"
		if i > 2 {
			stage = "implementation"
		}
		step := execution.Step{ID: name, Stage: stage, Routes: map[execution.Outcome]execution.StepID{}}
		if owners[i] == "" {
			step.Decision = &execution.DecisionSpec{Authority: "human"}
		} else {
			step.Run = &execution.RunSpec{Package: owners[i]}
		}
		if i < len(names)-1 {
			step.Routes["completed"] = names[i+1]
		} else {
			step.Routes["approve"] = execution.Complete
		}
		steps[i] = step
	}
	steps[1].Routes["changes_requested"] = "specification"
	steps[2].Routes = map[execution.Outcome]execution.StepID{"approve": "build", "request_changes": "specification"}
	steps[4].Routes["changes_requested"] = "build"
	record := control.Execution{ID: "preview-run", ContextID: "hello-go", Workflow: "Go coding", State: execution.ExecutionRunning, CreatedAt: now.Add(-242 * time.Second), CurrentStepID: "build-visit", Configuration: execution.Configuration{Workflow: execution.Workflow{Steps: steps, Stages: []execution.Stage{{ID: "design", Loop: &execution.LoopPolicy{Limit: 3}}, {ID: "implementation", Loop: &execution.LoopPolicy{Limit: 3}}}}}}
	for i, seconds := range []int{45, 19, 56} {
		start := now.Add(-time.Duration(240-i*50) * time.Second)
		end := start.Add(time.Duration(seconds) * time.Second)
		record.Steps = append(record.Steps, control.Step{ID: fmt.Sprintf("visit-%d", i), Key: names[i], State: "completed", Visit: 1, Try: 1, StartedAt: start, FinishedAt: &end})
	}
	record.Steps = append(record.Steps, control.Step{ID: "build-visit", Key: "build", State: "running", Visit: 2, Try: 1, StartedAt: now.Add(-90 * time.Second)})
	return model{executions: []control.Execution{record}, selectedStep: 3, follow: true, contextID: "*", connected: true}
}

func TestWorkflowShowsStageFramesAndExplicitRoutes(t *testing.T) {
	m := workflowExample()
	lines := m.workflowLines(124, 60)
	view := ansi.Strip(strings.Join(lines, "\n"))
	require.Contains(t, view, "DESIGN · sequential loop ≤3 rounds")
	require.Contains(t, view, "IMPLEMENTATION · sequential loop ≤3 rounds")
	require.Contains(t, view, "architecture_review ↺ specification · changes_requested")
	require.Contains(t, view, "design_approval ──approve──▶ build")
	require.Contains(t, view, "code_review ↺ build · changes_requested")
	require.Contains(t, view, "HUMAN DECISION")
	require.Contains(t, view, "Now: implementation / build · running")
	require.Contains(t, view, "Elapsed 4m2s")
	require.NotContains(t, view, "visit 2 · attempt 1", "selected-step details belong in Enter inspect, not below the graph")
	require.GreaterOrEqual(t, strings.Count(view, "╭"), 6)
	require.Contains(t, view, "╔")
	require.NotContains(t, view, "…")
}

func TestUnconnectedStepsHaveNoInventedArrow(t *testing.T) {
	m := workflowExample()
	steps := m.executions[0].Configuration.Workflow.Steps
	steps[0].Routes = nil
	diagram := m.stageDiagram(workflowStage{first: 0, end: 2, name: "design"}, 100, 0, time.Now(), false)
	require.NotContains(t, ansi.Strip(strings.Join(diagram, "\n")), "──▶")
	require.Empty(t, stageConnection(steps[0], steps[3], 80))
}

func TestNonadjacentAndCrossStageRoutesRemainVisible(t *testing.T) {
	m := workflowExample()
	m.executions[0].Configuration.Workflow.Steps[0].Routes["skip"] = "code_review"
	diagram := m.stageDiagram(workflowStage{first: 0, end: 3, name: "design"}, 124, 0, time.Now(), false)
	require.Contains(t, ansi.Strip(strings.Join(diagram, "\n")), "specification → code_review · skip")
}

func TestCardSelectionPreservesFailureColor(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	m := workflowExample()
	m.executions[0].Steps[3].State = "failed"
	card := m.stepCard(m.executions[0].Configuration.Workflow.Steps[3], 34, true, time.Now())
	require.Contains(t, card, "\x1b[38;2;239;150;150m")
	require.Contains(t, card, "38;2;166;191;255")
	require.Contains(t, ansi.Strip(card), "▸ build")
	require.Contains(t, ansi.Strip(card), "✕ failed")
	require.NotContains(t, ansi.Strip(card), "visit")
}

func TestWaitingHumanApprovalIsDistinct(t *testing.T) {
	m := workflowExample()
	m.executions[0].Steps = append(m.executions[0].Steps, control.Step{Key: "design_approval", State: "waiting_input", Visit: 2, Try: 1})
	card := ansi.Strip(m.stepCard(m.executions[0].Configuration.Workflow.Steps[2], 34, false, time.Now()))
	require.Contains(t, card, "HUMAN DECISION")
	require.Contains(t, card, "awaiting input")
	require.Contains(t, card, "╔")
	require.Contains(t, card, "◆")
}

func TestWrappedRowsConnectCorrectCardCenters(t *testing.T) {
	require.Equal(t, []string{"        │", "  ┌─────┘", "  ▼"}, rowConnection(8, 2))
	require.Equal(t, []string{"  │", "  └─────┐", "        ▼"}, rowConnection(2, 8))
	require.Equal(t, []string{"  │", "  ▼"}, rowConnection(2, 2))
}

func TestWorkflowAndPanelsStayInsideTerminal(t *testing.T) {
	for _, size := range [][2]int{{72, 18}, {80, 24}, {110, 30}, {160, 45}, {218, 70}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := workflowExample()
			m.width, m.height = size[0], size[1]
			m.activity = []state.Activity{{Source: "agent/architect/long-identity", Data: []byte(`{"output":"` + strings.Repeat("\t長いdiagnostic ", 50) + `"}`)}}
			for selected := range m.executions[0].Configuration.Workflow.Steps {
				m.selectedStep = selected
				p := layout(m.width, m.height)
				lines := m.workflowLines(p.center-4, p.body-3)
				require.LessOrEqual(t, len(lines), p.body-3)
				for _, line := range lines {
					require.LessOrEqual(t, ansi.StringWidth(line), p.center-4, "%q", ansi.Strip(line))
				}
				view := m.View()
				require.LessOrEqual(t, lipgloss.Height(view), m.height)
				require.LessOrEqual(t, lipgloss.Width(view), m.width)
			}
		})
	}
}

func TestNarrowWorkflowPreservesSelectedAgentName(t *testing.T) {
	m := workflowExample()
	m.selectedStep = 4
	lines := m.workflowLines(26, 30)
	text := ansi.Strip(strings.Join(lines, ""))
	require.Contains(t, strings.Join(strings.Fields(strings.NewReplacer("│", "", "║", "").Replace(text)), ""), "code_reviewer_golang", "narrow cards may wrap agent names, but must not omit them")
	require.NotContains(t, text, "code_reviewer_gola…")
}

func TestTerminalWorkflowDurationStopsAtCompletion(t *testing.T) {
	now := time.Now()
	record := control.Execution{CreatedAt: now.Add(-5 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute), State: execution.ExecutionCompleted}
	require.Equal(t, "3m0s", executionElapsed(&record, now))
	require.Equal(t, "3m0s", executionElapsed(&record, now.Add(time.Hour)))
	record.State = execution.ExecutionWaitingInput
	require.Equal(t, "5m0s", executionElapsed(&record, now))
}

func TestCompactStageKeepsReturnRouteAndHiddenStepIdentity(t *testing.T) {
	m := workflowExample()
	diagram := m.stageDiagram(workflowStage{first: 3, end: 6, name: "implementation", limit: 3}, 86, 4, time.Now(), true)
	text := ansi.Strip(strings.Join(diagram, "\n"))
	require.Contains(t, text, "Other steps: acceptance")
	require.Contains(t, text, "code_review ↺ build · changes_requested")
	require.NotContains(t, text, "…")
}

func TestActivityContinuationLinesKeepTheirDataColumn(t *testing.T) {
	item := state.Activity{Source: "agent/architect/long-session-name", At: time.Now(), Data: []byte(strings.Repeat("界\tfeedback ", 15) + "\nFINAL DIAGNOSTIC")}
	lines := activityLines(item, 90)
	prefixWidth := 8 + 1 + 28 + 2
	require.Greater(t, len(lines), 2)
	for _, line := range lines[1:] {
		require.True(t, strings.HasPrefix(ansi.Strip(line), strings.Repeat(" ", prefixWidth)))
		require.LessOrEqual(t, ansi.StringWidth(line), 90)
	}
	require.Contains(t, strings.Join(lines, "\n"), "FINAL DIAGNOSTIC")
}

func TestConcurrentWorkflowShowsEveryActiveMember(t *testing.T) {
	m := workflowExample()
	record := &m.executions[0]
	record.Configuration.Workflow.Stages[1].Loop.Mode = "concurrent"
	record.Steps = append(record.Steps, control.Step{ID: "review-visit", Key: "code_review", State: "running", Visit: 2, Try: 1, StartedAt: time.Now()})
	view := ansi.Strip(strings.Join(m.workflowLines(124, 60), "\n"))
	require.Contains(t, view, "concurrent loop")
	require.Contains(t, view, "Active: build (running) · code_review (running)")
}
