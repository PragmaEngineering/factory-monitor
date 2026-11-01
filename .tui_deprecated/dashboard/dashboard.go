// Package dashboard provides a live, execution-first view of Factory.
package dashboard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"controls/execution"
	"controls/internal/comms"
	"controls/internal/control"
	"controls/internal/sessions"
	"controls/internal/state"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/sync/errgroup"
)

var (
	accent = lipgloss.Color("#A6BFFF")
	dim    = lipgloss.Color("#A1A7B3")
	good   = lipgloss.Color("#86CFA3")
	warn   = lipgloss.Color("#E8C078")
	bad    = lipgloss.Color("#EF9696")
	bright = lipgloss.Color("#E4E7ED")
	border = lipgloss.Color("#444954")
)

type snapshot struct {
	conversation []comms.Message
	executions   []control.Execution
	sessions     []sessions.Session
	contexts     []control.Context
	contextID    string
	activity     []state.Activity
	output       sessions.Output
	messages     []sessions.Message
	executionID  string
	sessionID    string
	err          error
}
type tick struct{}
type action struct {
	message     string
	executionID string
	dialog      *dialog
	err         error
}

type model struct {
	composer          textarea.Model
	activitySpinner   spinner.Model
	animationRunning  bool
	rawLogs           bool
	logCache          []logEntry
	composerReady     bool
	composerDraftKey  string
	composerDrafts    map[string]string
	sending           bool
	composerError     string
	conversation      []comms.Message
	controller        *control.Client
	supervisor        *sessions.Client
	executions        []control.Execution
	sessions          []sessions.Session
	contexts          []control.Context
	contextID         string
	configPath        string
	preferredID       string
	selectedExecution int
	selectedStep      int
	sessionOffset     int
	selectedSessionID string
	inboxSelected     string
	inboxAgent        string
	stepPinned        bool
	page              monitorPage
	fullscreen        bool
	focus             int // 0 executions, 1 workflow/index, 2 logs, 3 inbox, 4 composer
	tab               int // 0 activity, 1 agent output, 2 workflow events
	activity          []state.Activity
	activityCursor    int64
	activityID        string
	output            string
	outputID          string
	messages          []sessions.Message
	scroll            int
	follow            bool
	dialog            *dialog
	notice            string
	connected         bool
	connectionError   string
	refreshing        bool
	width             int
	height            int
}

// Run opens Factory's interactive terminal interface. Leaving it does not
// stop an execution or an agent.
func Run(ctx context.Context, controller *control.Client, supervisor *sessions.Client) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	m := model{controller: controller, supervisor: supervisor, width: 120, height: 36, follow: true, refreshing: true,
		configPath: filepath.Join(cwd, "factory.json"), activitySpinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent))), animationRunning: true}
	_, err = tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func (m model) Init() tea.Cmd { return tea.Batch(m.refresh(), nextTick(), m.activitySpinner.Tick) }
func nextTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tick{} })
}

func (m model) refresh() tea.Cmd {
	controller, supervisor := m.controller, m.supervisor
	executionID, sessionID, cursor := m.executionID(), m.sessionID(), m.activityCursor
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result := snapshot{executionID: executionID, sessionID: sessionID}
		result.executions, result.err = controller.List(ctx, "")
		if result.err != nil {
			return result
		}
		selection, err := controller.GetContext(ctx)
		if err != nil {
			result.err = err
			return result
		}
		result.contextID = selection.ID
		result.contexts, result.err = controller.Contexts(ctx)
		if result.err != nil {
			return result
		}
		result.sessions, result.err = supervisor.List(ctx, executionID)
		if result.err != nil {
			return result
		}
		if executionID != "" {
			result.conversation, result.err = controller.Messages(ctx, executionID)
			if result.err != nil {
				return result
			}
			result.activity, result.err = controller.Stream(ctx, executionID, cursor)
			if result.err != nil {
				return result
			}
		}
		group, requests := errgroup.WithContext(ctx)
		group.SetLimit(4)
		if sessionID != "" {
			group.Go(func() error {
				var err error
				result.output, err = supervisor.Output(requests, sessionID, -1)
				return err
			})
		}
		batches := make([][]sessions.Message, len(result.sessions))
		for i, session := range result.sessions {
			if executionID == "" || session.ExecutionID != executionID {
				continue
			}
			group.Go(func() error {
				var err error
				batches[i], err = supervisor.Messages(requests, session.ID)
				return err
			})
		}
		result.err = group.Wait()
		for _, messages := range batches {
			result.messages = append(result.messages, messages...)
		}
		return result
	}
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.composerReady {
			m.composer.SetWidth(max(1, m.width-messageIndexWidth(m.width)-10))
		}
	case spinner.TickMsg:
		if !m.animating() {
			m.animationRunning = false
			return m, nil
		}
		var command tea.Cmd
		m.activitySpinner, command = m.activitySpinner.Update(msg)
		return m, command
	case tick:
		if m.refreshing {
			return m, nextTick()
		}
		m.refreshing = true
		commands := []tea.Cmd{m.refresh(), nextTick()}
		if m.animating() && !m.animationRunning {
			m.animationRunning = true
			commands = append(commands, m.activitySpinner.Tick)
		}
		return m, tea.Batch(commands...)
	case snapshot:
		m.refreshing = false
		if msg.err != nil {
			m.connected = false
			m.connectionError = msg.err.Error()
			return m, nil
		}
		m.connected, m.connectionError = true, ""
		oldID, oldStep := m.executionID(), m.stepKey()
		oldOutput := m.output
		m.executions, m.sessions = msg.executions, msg.sessions
		m.contexts, m.contextID = msg.contexts, msg.contextID
		if m.preferredID != "" && m.executionByID(m.preferredID) != nil {
			oldID, m.preferredID = m.preferredID, ""
			m.stepPinned = false
		}
		m.restoreSelection(oldID, oldStep)
		if m.page == messagesPage {
			known := false
			for _, recipient := range m.recipients() {
				known = known || recipient.key == m.inboxAgent
			}
			if !known {
				m.inboxAgent = "@operator"
			}
			if m.composerDraftKey != m.executionID()+"/"+m.inboxAgent {
				m.loadComposer()
			}
		}
		if oldID != m.executionID() {
			m.resetDetail()
		}
		if msg.executionID == m.executionID() {
			if m.activityID != msg.executionID {
				m.activity, m.activityCursor, m.activityID = nil, 0, msg.executionID
			}
			for _, item := range msg.activity {
				m.activityCursor = item.ID
				m.activity = append(m.activity, item)
			}
			if len(m.activity) > 2000 {
				m.activity = m.activity[len(m.activity)-2000:]
			}
			m.messages = msg.messages
			m.conversation = msg.conversation
		}
		if msg.sessionID == m.sessionID() {
			m.outputID = msg.sessionID
			m.output = msg.output.Text
		}
		if m.logCache == nil || len(msg.activity) > 0 || oldOutput != m.output {
			m.rebuildLogs()
		}
		if len(msg.activity) == 500 {
			m.refreshing = true
			return m, m.refresh()
		}
	case action:
		if msg.err != nil {
			m.notice = ""
			previous := m.dialog
			if previous == nil {
				previous = msg.dialog
			}
			title := "Operation failed"
			if msg.dialog != nil {
				title = msg.dialog.title + " failed"
			}
			m.dialog = errorDialog(title, msg.err.Error(), previous)
		} else {
			if m.dialog != nil {
				m.dialog.err = ""
				m.dialog.notice = msg.message
			} else {
				m.notice = msg.message
			}
			m.preferredID = msg.executionID
		}
		if m.refreshing {
			return m, nil
		}
		m.refreshing = true
		return m, m.refresh()
	case tea.MouseMsg:
		if m.dialog != nil {
			if msg.Button == tea.MouseButtonWheelUp {
				m.dialog.scroll -= 1
			} else if msg.Button == tea.MouseButtonWheelDown {
				m.dialog.scroll += 1
			}
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.move(-1)
		}
		if msg.Button == tea.MouseButtonWheelDown {
			m.move(1)
		}
	case deliveryResult:
		return m.finishMessage(msg)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.dialog != nil {
			return m.dialogKey(msg)
		}
		if m.page == messagesPage && m.focus == 4 {
			return m.composerKey(msg)
		}
		return m.navigationKey(msg)
	default:
		if m.page == messagesPage && m.focus == 4 && m.dialog == nil {
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(message)
			return m, cmd
		}
		if m.dialog != nil {
			command := m.dialog.updateInput(msg)
			return m, command
		}
	}
	return m, nil
}

func (m model) navigationKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "z":
		m.fullscreen = !m.fullscreen
	case "esc":
		m.fullscreen = false
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.dialog = helpDialog()
	case "a":
		m.dialog = m.actionsDialog()
	case "n":
		m.dialog = m.newDialog()
	case "i":
		m.focusInbox()
	case "g":
		if m.page != messagesPage {
			m.showPage(messagesPage)
		}
		m.focus = 1
	case "s":
		m.dialog = m.agentSessionsDialog()
	case "c":
		m.dialog = m.contextDialog()
	case "o":
		if m.focus == 3 {
			project, names := m.inboxReviewFiles()
			return m, openArtifacts(&control.Execution{Project: project}, names)
		}
	case "enter":
		switch m.focus {
		case 2:
			if m.rawLogs {
				m.notice = "Press v for readable entries, then Enter to expand."
			} else {
				m.dialog = m.logEntryDialog()
			}
		case 0:
			m.focus = 1
		case 1:
			if m.page == messagesPage {
				return m, m.focusComposer()
			}
			if request := m.pendingRequest(); request != nil && request.StepID == m.currentStepID() {
				m.dialog = m.answerDialog(request)
			} else if step := m.retryStep(); step != nil && step.ID == m.currentStepID() {
				m.dialog = m.retryDialog()
			} else {
				m.dialog = m.inspectDialog()
			}
		case 3:
			m.dialog = m.inboxItemDialog()
		}
	case "tab":
		m.cyclePane(1)
		if m.focus == 4 {
			return m, m.composer.Focus()
		}
	case "shift+tab":
		m.cyclePane(-1)
		if m.focus == 4 {
			return m, m.composer.Focus()
		}
	case "left":
		if m.focus == 1 {
			m.move(-1)
		}
	case "right":
		if m.focus == 1 {
			m.move(1)
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		for range 8 {
			m.move(-1)
		}
	case "pgdown":
		for range 8 {
			m.move(1)
		}
	case "home":
		if m.focus == 2 {
			m.scroll = 1 << 30
		} else {
			for m.move(-1) {
			}
		}
	case "end":
		if m.focus == 2 {
			m.scroll, m.follow = 0, true
		} else {
			for m.move(1) {
			}
		}
	case "1":
		m.showPage(homePage)
	case "2":
		m.showPage(messagesPage)
	case "3":
		m.showPage(logsPage)
	case "4":
		m.showPage(logsPage)
		m.tab, m.scroll, m.follow = 2, 0, true
		m.rebuildLogs()
	case "v":
		if m.page == logsPage {
			m.rawLogs = !m.rawLogs
			m.scroll = 0
			m.rebuildLogs()
		}
	case "l":
		if m.page != logsPage {
			m.showPage(logsPage)
		}
		m.tab, m.focus, m.scroll, m.follow = (m.tab+1)%3, 2, 0, true
		m.rebuildLogs()
	case "f":
		m.follow, m.scroll = true, 0
		m.stepPinned = false
		m.selectedSessionID = ""
		m.restoreSelection(m.executionID(), "")
	case "m":
		if m.page != messagesPage {
			target := string(m.stepKey())
			m.showPage(messagesPage)
			for _, recipient := range m.recipients() {
				if recipient.key == target {
					m.inboxAgent = target
					m.loadComposer()
					break
				}
			}
		}
		return m, m.focusComposer()
	case "[":
		m.logCache = nil
		m.scroll, m.follow = 0, true
		m.selectedSessionID = ""
		if step := m.selectedVisit(); step != nil && len(step.Sessions) > 0 {
			m.sessionOffset = min(len(step.Sessions)-1, m.sessionOffset+1)
		}
	case "]":
		m.logCache = nil
		m.scroll, m.follow = 0, true
		m.selectedSessionID = ""
		m.sessionOffset = max(0, m.sessionOffset-1)
	case "r":
		if m.refreshing {
			return m, nil
		}
		m.refreshing = true
		return m, m.refresh()
	}
	return m, nil
}

func (m *model) move(direction int) bool {
	switch m.focus {
	case 0:
		next := m.selectedExecution + direction
		if next < 0 || next >= len(m.executions) {
			return false
		}
		m.selectedExecution, m.selectedStep = next, 0
		m.stepPinned = false
		m.resetDetail()
	case 1:
		if m.page == messagesPage {
			return m.moveRecipient(direction)
		}
		next := m.selectedStep + direction
		if next < 0 || next >= len(m.steps()) {
			return false
		}
		m.selectedStep = next
		m.stepPinned = true
		m.sessionOffset = 0
		m.selectedSessionID = ""
		m.output, m.outputID = "", ""
	case 2:
		if !m.rawLogs {
			m.scroll = min(m.scroll, max(0, len(m.logRecords())-1))
		}
		if direction > 0 && m.scroll == 0 {
			return false
		}
		m.scroll = max(0, m.scroll-direction)
		m.follow = m.scroll == 0
	case 3:
		items := m.inboxItems()
		index := m.inboxIndex(items)
		if index < 0 {
			index = 0
		} else {
			index += direction
		}
		if index < 0 || index >= len(items) {
			return false
		}
		m.inboxSelected = items[index].id
	}
	return true
}

func (m *model) resetDetail() {
	m.activity, m.activityCursor, m.activityID = nil, 0, ""
	m.logCache = nil
	m.output, m.outputID, m.messages = "", "", nil
	m.sessionOffset = 0
	m.selectedSessionID, m.inboxSelected = "", ""
	m.scroll, m.follow = 0, true
}

func (m model) executionID() string {
	if m.selectedExecution < 0 || m.selectedExecution >= len(m.executions) {
		return ""
	}
	return m.executions[m.selectedExecution].ID
}
func (m model) currentExecution() *control.Execution {
	if m.executionID() == "" {
		return nil
	}
	return &m.executions[m.selectedExecution]
}
func (m model) steps() []execution.Step {
	if current := m.currentExecution(); current != nil {
		return current.Configuration.Workflow.Steps
	}
	return nil
}
func (m model) stepKey() execution.StepID {
	steps := m.steps()
	if m.selectedStep >= 0 && m.selectedStep < len(steps) {
		return steps[m.selectedStep].ID
	}
	return ""
}
func (m model) selectedVisit() *control.Step {
	current := m.currentExecution()
	if current == nil {
		return nil
	}
	for i := len(current.Steps) - 1; i >= 0; i-- {
		if current.Steps[i].Key == m.stepKey() {
			return &current.Steps[i]
		}
	}
	return nil
}
func (m model) currentStepID() string {
	if step := m.selectedVisit(); step != nil {
		return step.ID
	}
	return ""
}
func (m model) sessionID() string {
	if m.selectedSessionID != "" {
		for _, session := range m.sessions {
			if session.ID == m.selectedSessionID && session.ExecutionID == m.executionID() {
				return session.ID
			}
		}
	}
	step := m.selectedVisit()
	if step == nil || len(step.Sessions) == 0 {
		return ""
	}
	return step.Sessions[max(0, len(step.Sessions)-1-m.sessionOffset)]
}
func (m model) activeSession() *sessions.Session {
	id := m.sessionID()
	for i := range m.sessions {
		if m.sessions[i].ID == id {
			return &m.sessions[i]
		}
	}
	return nil
}
func (m *model) restoreSelection(executionID string, stepKey execution.StepID) {
	if m.page == logsPage {
		defer m.ensureAgentSelection()
	}
	for i := range m.executions {
		if m.executions[i].ID == executionID {
			m.selectedExecution = i
			break
		}
	}
	if m.selectedExecution >= len(m.executions) {
		m.selectedExecution = max(0, len(m.executions)-1)
	}
	steps := m.steps()
	if m.stepPinned {
		for i := range steps {
			if steps[i].ID == stepKey {
				m.selectedStep = i
				return
			}
		}
	}
	if current := m.currentExecution(); current != nil {
		for i := range steps {
			for _, visit := range current.Steps {
				if visit.ID == current.CurrentStepID && visit.Key == steps[i].ID {
					m.selectedStep = i
					return
				}
			}
		}
	}
	m.selectedStep = 0
}

func (m model) executionLines(width, height int) []string {
	if len(m.executions) == 0 {
		return []string{"No executions yet.", "", "Press n to start one."}
	}
	visible := max(1, height/3)
	start := max(0, min(m.selectedExecution-visible/2, len(m.executions)-visible))
	end := min(len(m.executions), start+visible)
	lines := make([]string, 0, (end-start)*3)
	for i := start; i < end; i++ {
		item := m.executions[i]
		marker := "  "
		if i == m.selectedExecution {
			marker = "▸ "
		}
		name := item.Brief
		if name == "" && len(item.Input.Documents) > 0 {
			name = item.Input.Documents[0].Name
		}
		if name == "" {
			name = item.Workflow
		}
		lines = append(lines, selectedRow(marker+shortID(item.ID)+"  "+statusGlyph(string(item.State))+" "+string(item.State), width, i == m.selectedExecution, stateColor(string(item.State))))
		lines = append(lines, lipgloss.NewStyle().Foreground(dim).Render("  "+ansi.Truncate(terminalLine(name), max(1, width-2), "…")))
		lines = append(lines, "")
	}
	return lines
}

func (m model) detailTitle() string {
	labels := []string{"LOGS · ALL ACTIVITY", "LOGS · AGENT OUTPUT", "LOGS · WORKFLOW EVENTS"}
	label := labels[m.tab]
	if m.tab == 0 {
		label += " · " + fmt.Sprint(len(m.activity)) + " events"
	} else if agent := m.activeSession(); agent != nil && m.tab == 1 {
		label += "  ·  " + agent.Name + " / " + string(agent.State)
		if step := m.selectedVisit(); step != nil && len(step.Sessions) > 1 {
			label += fmt.Sprintf("  ·  %d/%d", max(1, len(step.Sessions)-m.sessionOffset), len(step.Sessions))
		}
	}
	if m.tab == 1 {
		label += fmt.Sprintf(" · %d running · s select", m.runningAgents())
	}
	if !m.follow {
		label += "  ·  paused"
	}
	if m.rawLogs {
		label += " · RAW"
	} else {
		label += " · READABLE · Enter expand"
	}
	return label
}
func activityLines(item state.Activity, width int) []string {
	data := strings.TrimSuffix(terminalText(string(item.Data)), "\n")
	parts := strings.Split(data, "\n")
	sourceWidth := min(28, max(8, width/3))
	prefixWidth := 8 + 1 + sourceWidth + 2
	if width < prefixWidth+8 {
		return strings.Split(ansi.Hardwrap(item.At.Local().Format("15:04:05")+" "+terminalLine(item.Source)+" "+data, max(1, width), true), "\n")
	}
	source := ansi.Truncate(terminalLine(item.Source), sourceWidth, "…")
	source += strings.Repeat(" ", max(0, sourceWidth-ansi.StringWidth(source)))
	prefix := lipgloss.NewStyle().Foreground(dim).Render(item.At.Local().Format("15:04:05")) + " " +
		lipgloss.NewStyle().Foreground(accent).Render(source) + "  "
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		for _, fragment := range strings.Split(ansi.Hardwrap(part, width-prefixWidth, true), "\n") {
			if len(lines) == 0 {
				lines = append(lines, prefix+fragment)
			} else {
				lines = append(lines, strings.Repeat(" ", prefixWidth)+fragment)
			}
		}
	}
	return lines
}
func tint(value string, color lipgloss.Color, selected bool) string {
	style := lipgloss.NewStyle().Foreground(color)
	if selected {
		style = style.Bold(true).Background(selection)
	}
	return style.Render(value)
}
func stateColor(value string) lipgloss.Color {
	switch value {
	case "completed", "submitted", "exited":
		return good
	case "blocked", "failed", "interrupted", "cancelled":
		return bad
	case "waiting_input", "stopping":
		return warn
	case "running":
		return accent
	default:
		return dim
	}
}
func statusGlyph(value string) string {
	switch value {
	case "completed", "submitted":
		return "✓"
	case "running":
		return "◉"
	case "blocked", "failed":
		return "✕"
	case "waiting_input":
		return "◆"
	default:
		return "○"
	}
}
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func (m model) animating() bool {
	return (!m.connected && m.connectionError == "") || m.sending || m.runningAgents() > 0
}
func (m model) runningGlyph(status string) string {
	if status == "running" && m.animationRunning {
		return m.activitySpinner.View()
	}
	return statusGlyph(status)
}
