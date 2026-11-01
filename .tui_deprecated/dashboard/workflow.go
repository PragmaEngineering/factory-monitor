package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"controls/execution"
	"controls/internal/control"
	"controls/internal/sessions"
	"controls/internal/telemetry"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// A stage is a visual group, not an inferred dependency. Only configured routes
// draw arrows; adjacency in the configuration never implies a connection.
type workflowStage struct {
	first, end int
	name       string
	limit      uint
	mode       string
}

func (m model) workflowLines(width, height int) []string {
	current := m.currentExecution()
	if current == nil {
		return []string{"Select an execution to inspect its workflow."}
	}
	if width < 1 || height < 1 {
		return nil
	}
	now := time.Now()
	steps := current.Configuration.Workflow.Steps
	stages := workflowStages(current.Configuration.Workflow)
	lines := m.workflowSummary(width, now)
	if len(steps) == 0 {
		return append(lines, "No steps configured.")
	}
	selected := max(0, min(m.selectedStep, len(steps)-1))
	available := max(0, height-len(lines))
	var graph []string
	var diagrams [][]string
	selectedStage := 0
	for i, stage := range stages {
		if selected >= stage.first && selected < stage.end {
			selectedStage = i
		}
		diagram := m.stageDiagram(stage, width, selected, now, false)
		diagrams = append(diagrams, diagram)
		if i > 0 {
			graph = append(graph, stageConnection(steps[stages[i-1].end-1], steps[stage.first], width)...)
		}
		graph = append(graph, diagram...)
	}

	if len(graph) > available {
		// Prefer the selected stage, then spend remaining space on its neighbors.
		// Keep configuration order; collapse complete frames rather than crop them.
		chosen := make([][]string, len(stages))
		for i, stage := range stages {
			chosen[i] = []string{m.stageSummary(stage, width)}
		}
		chosen[selectedStage] = diagrams[selectedStage]
		total := len(stages) - 1 + len(chosen[selectedStage])
		if total > available {
			chosen[selectedStage] = m.stageDiagram(stages[selectedStage], width, selected, now, true)
			total = len(stages) - 1 + len(chosen[selectedStage])
		}
		for distance := 1; distance < len(stages); distance++ {
			for _, i := range []int{selectedStage - distance, selectedStage + distance} {
				if i < 0 || i >= len(stages) {
					continue
				}
				extra := len(diagrams[i]) - len(chosen[i])
				if total+extra <= available {
					chosen[i] = diagrams[i]
					total += extra
				}
			}
		}
		graph = nil
		for i, diagram := range chosen {
			if i > 0 {
				connection := stageConnection(steps[stages[i-1].end-1], steps[stages[i].first], width)
				if total+len(connection) <= available {
					graph = append(graph, connection...)
					total += len(connection)
				}
			}
			graph = append(graph, diagram...)
		}
	}
	if len(graph) > available {
		// Very short terminals cannot fit nested boxes. Prefer explicit stage
		// and step identities over clipped borders or half-visible nodes.
		graph = []string{m.stageSummary(stages[selectedStage], width)}
		stage := stages[selectedStage]
		for i := stage.first; i < stage.end; i++ {
			marker := "  "
			if i == selected {
				marker = "▸ "
			}
			status, _, _ := m.stepPresentation(steps[i], now)
			graph = append(graph, tint(ansi.Truncate(marker+statusGlyph(status)+" "+string(steps[i].ID), width, "…"), stateColor(status), i == selected))
		}
		if len(graph) > available {
			graph = []string{m.stageSummary(stages[selectedStage], width)}
		}
	}
	lines = append(lines, graph...)
	return lines[:min(len(lines), height)]
}

func workflowStages(workflow execution.Workflow) []workflowStage {
	var stages []workflowStage
	for first := 0; first < len(workflow.Steps); {
		name := workflow.Steps[first].Stage
		end := first + 1
		for end < len(workflow.Steps) && workflow.Steps[end].Stage == name {
			end++
		}
		stage := workflowStage{first: first, end: end, name: name}
		if stage.name == "" {
			stage.name = "workflow"
		}
		for _, policy := range workflow.Stages {
			if policy.ID == name && policy.Loop != nil {
				stage.limit = policy.Loop.Limit
				stage.mode = policy.Loop.Mode
				if stage.mode == "" {
					stage.mode = "sequential"
				}
			}
		}
		stages = append(stages, stage)
		first = end
	}
	return stages
}

func (m model) workflowSummary(width int, now time.Time) []string {
	current := m.currentExecution()
	summary := fmt.Sprintf("Elapsed %s · %s · %s", executionElapsed(current, now), current.State, current.Workflow)
	var active []string
	for _, visit := range current.Steps {
		if visit.State == "running" || visit.State == "waiting_input" || visit.State == "blocked" || visit.State == "waiting_peer" {
			active = append(active, fmt.Sprintf("%s (%s)", visit.Key, visit.State))
		}
	}
	var stageNames []string
	for _, stage := range workflowStages(current.Configuration.Workflow) {
		stageNames = append(stageNames, strings.ToUpper(stage.name))
	}
	progress := strings.Join(stageNames, " · ")
	for _, visit := range current.Steps {
		if visit.ID != "" && visit.ID == current.CurrentStepID {
			progress = fmt.Sprintf("Now: %s · %s", visit.Key, visit.State)
			for _, step := range current.Configuration.Workflow.Steps {
				if step.ID == visit.Key && step.Stage != "" {
					progress = "Now: " + step.Stage + " / " + string(visit.Key) + " · " + visit.State
				}
			}
			break
		}
	}
	if len(active) > 1 {
		progress = "Active: " + strings.Join(active, " · ")
	}
	usage := telemetry.Summary{}
	for _, session := range m.sessions {
		if session.ExecutionID == current.ID && session.Runner != "hook" {
			usage.Include(session.Resources)
		}
	}
	resource := fmt.Sprintf("Reported tokens %s · tool failures %s · cost %s", compactCount(usage.Tokens.Total()), usage.Failures(), usage.Cost())
	if usage.Samples == 0 {
		resource = "Usage unreported · cost unknown"
	}
	if usage.UnreportedSessions > 0 {
		resource += fmt.Sprintf(" · %d unreported sessions", usage.UnreportedSessions)
	}
	lines := []string{ansi.Truncate(summary, width, "…"), tint(ansi.Truncate(progress, width, "…"), dim, false), tint(ansi.Truncate(resource, width, "…"), dim, false)}
	if len(current.Hooks) > 0 {
		succeeded := 0
		for _, hook := range current.Hooks {
			if hook.State == "succeeded" {
				succeeded++
			}
		}
		text := fmt.Sprintf("Hooks %d/%d complete", succeeded, len(current.Hooks))
		color := dim
		for _, hook := range current.Hooks {
			if hook.ID == current.CurrentStepID {
				text += fmt.Sprintf(" · %s/%s %s · %s", hook.Context.Scope, hook.Context.Boundary, hook.Context.Name, hook.State)
				if hook.Error != "" {
					text += " · " + terminalLine(hook.Error)
				}
				color = warn
			}
		}
		lines = append(lines, tint(ansi.Truncate(text, width, "…"), color, false))
	}
	return lines
}

func (m model) stageHeading(stage workflowStage) string {
	heading := strings.ToUpper(terminalLine(stage.name))
	if stage.limit > 0 {
		heading += fmt.Sprintf(" · %s loop ≤%d rounds", stage.mode, stage.limit)
	}
	return heading
}

func (m model) stageSummary(stage workflowStage, width int) string {
	steps := m.currentExecution().Configuration.Workflow.Steps
	completed := 0
	for _, step := range steps[stage.first:stage.end] {
		visit := m.latestWorkflowVisit(step.ID)
		if visit != nil && visit.State == "completed" {
			completed++
		}
	}
	text := fmt.Sprintf("%s · %d/%d complete", m.stageHeading(stage), completed, stage.end-stage.first)
	return tint(ansi.Truncate(text, width, "…"), dim, false)
}

func (m model) stageDiagram(stage workflowStage, width, selected int, now time.Time, selectedOnly bool) []string {
	steps := m.currentExecution().Configuration.Workflow.Steps
	inner := max(1, width-4) // outer border and one cell of padding on each side
	desired := 28
	for _, step := range steps[stage.first:stage.end] {
		desired = max(desired, ansi.StringWidth(terminalLine(string(step.ID)))+6, ansi.StringWidth(stepOwner(step))+4)
	}
	columns := max(1, min(3, (inner+3)/(desired+3)))
	columns = min(columns, stage.end-stage.first)
	cardWidth := min(42, (inner-(columns-1)*3)/columns)
	var content []string
	heading := m.stageHeading(stage)
	content = append(content, tint(ansi.Truncate(heading, inner, "…"), accent, true))
	for row := stage.first; row < stage.end; row += columns {
		end := min(stage.end, row+columns)
		isSelectedRow := selected >= row && selected < end
		if selectedOnly && !isSelectedRow {
			continue
		}
		var cards []string
		for i := row; i < end; i++ {
			cards = append(cards, m.stepCard(steps[i], cardWidth, selected == i, now))
		}
		rowHeight := 0
		for _, card := range cards {
			rowHeight = max(rowHeight, lipgloss.Height(card))
		}
		var connected []string
		for i, card := range cards {
			if i > 0 {
				connector := "   "
				if stage.mode != "concurrent" && routesTo(steps[row+i-1], steps[row+i].ID) {
					connector = tint("──▶", dim, false)
				}
				connected = append(connected, strings.Repeat("\n", rowHeight/2)+connector)
			}
			connected = append(connected, lipgloss.NewStyle().Height(rowHeight).Render(card))
		}
		rowWidth := len(cards)*cardWidth + (len(cards)-1)*3
		indent := max(0, (inner-rowWidth)/2)
		rowLines := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, connected...), "\n")
		for i := range rowLines {
			rowLines[i] = strings.Repeat(" ", indent) + rowLines[i]
		}
		content = append(content, rowLines...)
		if stage.mode != "concurrent" && !selectedOnly && end < stage.end && routesTo(steps[end-1], steps[end].ID) {
			nextCount := min(columns, stage.end-end)
			nextWidth := nextCount*cardWidth + (nextCount-1)*3
			source := indent + (len(cards)-1)*(cardWidth+3) + cardWidth/2
			target := (inner-nextWidth)/2 + cardWidth/2
			content = append(content, rowConnection(source, target)...)
		}
	}
	if selectedOnly {
		firstVisible := stage.first + (selected-stage.first)/columns*columns
		var hidden []string
		for i := stage.first; i < stage.end; i++ {
			if i < firstVisible || i >= firstVisible+columns {
				hidden = append(hidden, string(steps[i].ID))
			}
		}
		if len(hidden) > 0 {
			content = append(content, tint(ansi.Truncate("Other steps: "+strings.Join(hidden, ", ")+" · ↑↓ select", inner, "…"), dim, false))
		}
	}
	// Return and branching routes are explicitly labeled. We do not invent
	// verdicts, loop iterations, or edges from neighboring boxes.
	for i := stage.first; i < stage.end; i++ {
		for _, route := range sortedRoutes(steps[i]) {
			if i+1 < stage.end && route.target == steps[i+1].ID {
				continue
			}
			// The boundary transition has its own labeled connector.
			if i == stage.end-1 && stage.end < len(steps) && route.target == steps[stage.end].ID {
				continue
			}
			for j := range steps {
				if route.target != steps[j].ID {
					continue
				}
				arrow := "→"
				if j <= i {
					arrow = "↺"
				}
				label := fmt.Sprintf("%s %s %s · %s", steps[i].ID, arrow, route.target, route.outcome)
				content = append(content, wrapWorkflowText(label, inner, dim)...)
			}
		}
	}
	frame := lipgloss.NewStyle().Width(max(1, width-2)).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(border).Background(surface).BorderBackground(surface)
	return strings.Split(frame.Render(strings.Join(content, "\n")), "\n")
}

// rowConnection leaves the source card's center and enters the destination
// card's center, including when a row wraps in either direction.
func rowConnection(source, target int) []string {
	if source == target {
		return []string{strings.Repeat(" ", source) + "│", strings.Repeat(" ", target) + "▼"}
	}
	elbow := ""
	if source > target {
		elbow = strings.Repeat(" ", target) + "┌" + strings.Repeat("─", source-target-1) + "┘"
	} else {
		elbow = strings.Repeat(" ", source) + "└" + strings.Repeat("─", target-source-1) + "┐"
	}
	return []string{strings.Repeat(" ", source) + "│", elbow, strings.Repeat(" ", target) + "▼"}
}

func stageConnection(source, target execution.Step, width int) []string {
	var lines []string
	for _, route := range sortedRoutes(source) {
		if route.target == target.ID {
			label := fmt.Sprintf("%s ──%s──▶ %s", source.ID, route.outcome, target.ID)
			lines = append(lines, wrapWorkflowText(label, width, dim)...)
		}
	}
	return lines
}

type workflowRoute struct {
	outcome string
	target  execution.StepID
}

func sortedRoutes(step execution.Step) []workflowRoute {
	var routes []workflowRoute
	for outcome, target := range step.Routes {
		routes = append(routes, workflowRoute{string(outcome), target})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].outcome < routes[j].outcome })
	return routes
}

func routesTo(step execution.Step, target execution.StepID) bool {
	for _, destination := range step.Routes {
		if destination == target {
			return true
		}
	}
	return false
}

func stepOwner(step execution.Step) string {
	if step.Decision != nil {
		authority := strings.ToUpper(terminalLine(step.Decision.Authority))
		if authority != "" {
			return authority + " DECISION"
		}
		return "DECISION"
	}
	if step.Run != nil {
		return terminalLine(step.Run.Package)
	}
	return "agent"
}

func (m model) latestWorkflowVisit(id execution.StepID) *control.Step {
	current := m.currentExecution()
	if current == nil {
		return nil
	}
	for i := len(current.Steps) - 1; i >= 0; i-- {
		if current.Steps[i].Key == id {
			return &current.Steps[i]
		}
	}
	return nil
}

func (m model) stepPresentation(step execution.Step, now time.Time) (status, label, elapsed string) {
	status = "pending"
	if visit := m.latestWorkflowVisit(step.ID); visit != nil {
		status = visit.State
		end := now
		if visit.FinishedAt != nil {
			end = *visit.FinishedAt
		}
		if !visit.StartedAt.IsZero() {
			elapsed = duration(end.Sub(visit.StartedAt))
		}
	}
	label = status
	if status == "waiting_input" {
		label = "awaiting input"
	}
	return status, label, elapsed
}

func (m model) stepCard(step execution.Step, width int, selected bool, now time.Time) string {
	status, label, elapsed := m.stepPresentation(step, now)
	color := stateColor(status)
	inner := max(1, width-4)
	title := terminalLine(string(step.ID))
	marker := "  "
	if selected {
		marker = "▸ "
	}
	rows := wrapWorkflowText(strings.TrimSpace(m.runningGlyph(status)+" "+label), inner, color)
	titleRow := wrapWorkflowText(marker+title, inner, bright)
	if step.Title != "" && step.Title != title {
		titleRow = append(titleRow, wrapWorkflowText(step.Title, inner, bright)...)
	}
	if selected {
		titleRow[0] = tint(ansi.Strip(titleRow[0]), accent, true)
	}
	rows = append(rows, titleRow...)
	ownerColor := dim
	if step.Decision != nil {
		ownerColor = warn
	}
	rows = append(rows, wrapWorkflowText(stepOwner(step), inner, ownerColor)...)
	detail := "Elapsed " + elapsed
	if elapsed == "" {
		detail = "Not started"
	}
	if visit := m.latestWorkflowVisit(step.ID); visit != nil && visit.Try > 0 {
		detail += fmt.Sprintf(" · try %d", visit.Try)
	}
	rows = append(rows, wrapWorkflowText(detail, inner, dim)...)
	if visit := m.latestWorkflowVisit(step.ID); visit != nil && step.Run != nil {
		usage := telemetry.Summary{}
		for _, session := range m.sessions {
			if session.StepID != visit.ID {
				continue
			}
			usage.Include(session.Resources)
			if session.State == sessions.Running {
				progress := session.StartedAt
				if session.Resources.LastProgressAt != nil {
					progress = *session.Resources.LastProgressAt
				}
				rows = append(rows, wrapWorkflowText("Since progress "+duration(now.Sub(progress)), inner, dim)...)
				if command := session.Resources.Command(); command != "" {
					rows = append(rows, wrapWorkflowText("Running: "+ansi.Truncate(terminalLine(command), max(1, inner*2-9), "…"), inner, bright)...)
				}
			}
			if len(session.BudgetNotices) > 0 {
				rows = append(rows, wrapWorkflowText(session.BudgetNotices[len(session.BudgetNotices)-1].Message, inner, warn)...)
			}
		}
		label := "Tokens unknown"
		if usage.Samples > 0 {
			label = fmt.Sprintf("Tokens %s · in %s · cache R/W %s/%s · out %s", compactCount(usage.Tokens.Total()), compactCount(usage.Tokens.Input), compactCount(usage.Tokens.CacheRead), compactCount(usage.Tokens.CacheWrite), compactCount(usage.Tokens.Output))
		}
		rows = append(rows, wrapWorkflowText(label, inner, dim)...)
		rows = append(rows, wrapWorkflowText(fmt.Sprintf("Tool failures %s · cost %s", usage.Failures(), usage.Cost()), inner, dim)...)
	}
	cardBorder := lipgloss.RoundedBorder()
	if step.Decision != nil {
		cardBorder = lipgloss.DoubleBorder()
	}
	background := surface
	if selected {
		background = selection
	}
	return lipgloss.NewStyle().Width(max(1, width-2)).Padding(0, 1).Background(background).
		Border(cardBorder).BorderForeground(color).BorderBackground(surface).Render(strings.Join(rows, "\n"))
}

func wrapWorkflowText(value string, width int, color lipgloss.Color) []string {
	var lines []string
	for _, line := range strings.Split(ansi.Hardwrap(terminalLine(value), max(1, width), true), "\n") {
		lines = append(lines, tint(line, color, false))
	}
	return lines
}

func executionElapsed(record *control.Execution, now time.Time) string {
	if record.CreatedAt.IsZero() {
		return "—"
	}
	end := now
	switch record.State {
	case execution.ExecutionCompleted, execution.ExecutionFailed, execution.ExecutionCancelled:
		end = record.UpdatedAt
	}
	return duration(end.Sub(record.CreatedAt))
}

func duration(value time.Duration) string {
	return max(time.Duration(0), value).Truncate(time.Second).String()
}

func compactCount(n uint64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}
