package dashboard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"controls/execution"
	"controls/internal/control"
	"controls/internal/sessions"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type field struct {
	label string
	hint  string
	input textinput.Model
}

func inputField(label, value, hint string) field {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "…"
	input.CharLimit = 16384
	input.Cursor.SetMode(cursor.CursorStatic)
	input.SetValue(value)
	input.CursorEnd()
	return field{label: label, hint: hint, input: input}
}

type dialog struct {
	kind        string
	title       string
	detail      string
	rawDetail   string
	markdown    bool
	raw         bool
	renderWidth int
	renderRaw   bool
	rendered    []string
	options     []string
	selected    int
	fields      []field
	field       int
	request     *control.Request
	contexts    []control.Context
	sessions    []sessions.Session
	filesRoot   string
	targetID    string
	artifacts   []string
	scroll      int
	err         string
	notice      string
	previous    *dialog
}

func errorDialog(title, message string, previous *dialog) *dialog {
	return &dialog{kind: "error", title: title, detail: message, previous: previous}
}

func helpDialog() *dialog {
	return &dialog{kind: "help", title: "Keyboard guide", options: []string{
		"n  New execution (brief, files, config)",
		"a  Actions available for the selected execution",
		"i  Focus the inbox (decisions and messages)",
		"g  Open participant index   s  Select agent logs",
		"c  Select context",
		"Enter  Inspect workflow / compose for selected agent / read message",
		"1  Home   2  Messages   3  Logs",
		"l  Cycle logs: all activity / selected agent / workflow events",
		"o  Open listed files   c  Copy locations (inside an item dialog)",
		"Green LIVE means service connected, not that an agent is busy",
		"Elapsed time includes time waiting for input",
		"Tab / Shift-Tab  Change pane",
		"z  Expand focused pane / restore layout; Esc restores layout",
		"← →  Select workflow step; move cursor in a text field",
		"↑ ↓ or j k  Move selection; PgUp/PgDn scroll detail",
		"f  Follow the current step and newest output",
		"[ ]  Switch between agent sessions for this step",
		"m  Compose; Enter sends; Ctrl-J adds a newline; Esc leaves typing",
		"v  Readable/raw logs; Enter expands selected readable entry",
		"v  Readable/raw content inside message or log inspection",
		"r  Refresh   q  Quit (execution continues)",
	}}
}

func (m model) actionsDialog() *dialog {
	items := []string{"New execution", "Inbox", "Select context"}
	if current := m.currentExecution(); current != nil {
		if m.pendingRequest() != nil {
			items = append(items, "Answer decision")
		}
		if m.retryStep() != nil || m.retryHook() != nil {
			items = append(items, "Retry blocked operation")
		}
		if agent := m.activeSession(); agent != nil && agent.State == sessions.Running {
			if agent.InputMode != "none" {
				items = append(items, "Message agent")
			}
			items = append(items, "Stop agent session")
		}
		if m.retryHook() != nil {
			items = append(items, "Inspect hook")
		}
		items = append(items, "Inspect step")
		if cancellable(current.State) {
			items = append(items, "Cancel execution")
		}
	}
	return &dialog{kind: "actions", title: "Actions", detail: "Choose an operation for the selected execution.", options: items}
}

func (m model) newDialog() *dialog {
	return &dialog{kind: "new", title: "New execution", detail: "Choose a workflow configuration and provide its input.",
		fields: []field{
			inputField("Brief", "", "A short request; optional when files are given"),
			inputField("Input files", "", "Optional paths, separated by commas"),
			inputField("Config", m.configPath, "Path to project factory.json"),
		}}
}

func (m model) contextDialog() *dialog {
	items := []string{"*  All contexts"}
	selected := 0
	for i, item := range m.contexts {
		items = append(items, item.ID+"  "+item.Description)
		if item.ID == m.contextID {
			selected = i + 1
		}
	}
	return &dialog{kind: "contexts", title: "Context", detail: "The selected context filters executions and future runs.", options: items, selected: selected, contexts: append([]control.Context(nil), m.contexts...)}
}

func (m model) inspectDialog() *dialog {
	current, step := m.currentExecution(), m.selectedVisit()
	if current == nil {
		return &dialog{kind: "inspect", title: "No execution selected"}
	}
	detail := fmt.Sprintf("Execution %s\nContext %s · %s\nProject %s\nConfig %s\nState %s",
		current.ID, current.ContextID, current.Brief, current.Project, current.ConfigPath, current.State)
	if current.Error != "" {
		detail += "\nError: " + current.Error
	}
	if step != nil {
		detail += fmt.Sprintf("\n\nStep %s · %s\nState %s · visit %d · try %d", step.ID, step.Key, step.State, step.Visit, step.Try)
		if step.Error != "" {
			detail += "\nError: " + step.Error
		}
		if len(step.Sessions) > 0 {
			detail += "\nAgent session " + step.Sessions[len(step.Sessions)-1]
		}
	}
	return &dialog{kind: "inspect", title: "Execution detail", detail: detail}
}

func (m model) answerDialog(request *control.Request) *dialog {
	if request == nil {
		return nil
	}
	choices := make([]string, len(request.Options))
	for i, option := range request.Options {
		choices[i] = string(option)
	}
	if len(choices) == 0 {
		choices = []string{"answer"}
	}
	label, title, placeholder := "Reason", "Decision", "Optional context or feedback for the selected outcome"
	if request.Kind == "step_input" {
		title = "Agent input"
		if len(request.Options) == 0 {
			label = "Answer"
			placeholder = "Your response to the agent's question"
		}
	}
	current := m.executionByID(request.ExecutionID)
	return &dialog{kind: "answer", title: title, detail: request.Prompt, request: request,
		options: choices, fields: []field{inputField(label, "", placeholder)},
		artifacts: reviewArtifacts(current, request), filesRoot: executionFilesRoot(current)}
}

func (m model) retryDialog() *dialog {
	step := m.retryStep()
	if hook := m.retryHook(); hook != nil {
		return &dialog{kind: "retry", title: "Retry blocked hook", targetID: hook.ID, detail: hook.Context.Name + " · " + hook.Error, fields: []field{inputField("Reason", "", "What changed before retrying?")}}
	}
	if step == nil {
		return nil
	}
	return &dialog{kind: "retry", title: "Retry blocked step", targetID: step.ID,
		detail: fmt.Sprintf("%s · %s\n%s", step.Key, step.ID, step.Error),
		fields: []field{inputField("Reason", "", "What changed before retrying?")}}
}

func (m model) messageDialog() *dialog {
	agent := m.activeSession()
	if agent == nil || agent.State != sessions.Running {
		return nil
	}
	return &dialog{kind: "message", title: "Message agent", targetID: agent.ID,
		detail: agent.Name + " · " + agent.ID + "\nThe message is delivered to this session; delivery alone does not prove the agent acted on it.",
		fields: []field{inputField("Message", "", "Type a short message or supply a file below"),
			inputField("File", "", "Optional path to a text file (maximum 16 KiB)")}}
}

func (m model) cancelDialog() *dialog {
	if m.currentExecution() == nil || !cancellable(m.currentExecution().State) {
		return nil
	}
	return &dialog{kind: "cancel", title: "Cancel execution", targetID: m.executionID(),
		detail: "Cancel " + m.executionID() + "? This stops its active work.", options: []string{"No, keep running", "Yes, cancel"}}
}

func (m model) stopAgentDialog() *dialog {
	agent := m.activeSession()
	if agent == nil || agent.State != sessions.Running {
		return nil
	}
	return &dialog{kind: "stop-agent", title: "Stop agent session", targetID: agent.ID,
		detail:  "Stop " + agent.Name + " · " + agent.ID + "? The workflow may block until you retry the step.",
		options: []string{"No, keep running", "Yes, stop agent"}}
}

func cancellable(state execution.ExecutionState) bool {
	return state != execution.ExecutionCompleted && state != execution.ExecutionCancelled && state != execution.ExecutionFailed
}

func (m model) pendingRequest() *control.Request {
	if current := m.currentExecution(); current != nil {
		for i := range current.Requests {
			if current.Requests[i].State == "pending" {
				return &current.Requests[i]
			}
		}
	}
	return nil
}
func (m model) retryStep() *control.Step {
	current := m.currentExecution()
	if current == nil || current.State != execution.ExecutionBlocked {
		return nil
	}
	for i := range current.Steps {
		if current.Steps[i].ID == current.CurrentStepID && current.Steps[i].State == "blocked" {
			return &current.Steps[i]
		}
	}
	return nil
}
func (m model) executionByID(id string) *control.Execution {
	for i := range m.executions {
		if m.executions[i].ID == id {
			return &m.executions[i]
		}
	}
	return nil
}

func (m model) dialogKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.dialog
	if len(d.artifacts) > 0 && (d.kind != "answer" || d.field == 0) {
		switch key.String() {
		case "o":
			return m, openArtifacts(&control.Execution{Project: d.filesRoot}, d.artifacts)
		case "c":
			return m, copyReviewFiles(d.filesRoot, d.artifacts)
		}
	}
	readOnly := d.kind == "help" || d.kind == "inspect" || d.kind == "error"
	if readOnly {
		d.scroll = max(0, min(d.scroll, m.dialogScrollLimit()))
	}
	if key.String() == "esc" {
		m.dialog = d.previous
		return m, nil
	}
	if key.String() == "pgup" {
		d.scroll -= 6
		return m, nil
	}
	if key.String() == "pgdown" {
		d.scroll += 6
		return m, nil
	}
	if readOnly {
		switch key.String() {
		case "v":
			if d.rawDetail != "" {
				d.raw = !d.raw
				d.scroll = 0
			}
		case "up", "k":
			d.scroll = max(0, d.scroll-1)
		case "down", "j":
			d.scroll++
		case "home":
			d.scroll = 0
		case "end":
			d.scroll = m.dialogScrollLimit()
		case "enter":
			m.dialog = d.previous
		}
		return m, nil
	}
	if d.kind == "answer-confirm" || d.kind == "retry-confirm" {
		if key.String() == "y" {
			return m.submitDialog()
		}
		if key.String() == "n" || key.String() == "backspace" {
			d.kind = strings.TrimSuffix(d.kind, "-confirm")
		}
		return m, nil
	}
	switch d.kind {
	case "actions", "contexts", "cancel", "stop-agent", "agents":
		switch key.String() {
		case "up", "k":
			d.selected = max(0, d.selected-1)
		case "down", "j":
			d.selected = min(len(d.options)-1, d.selected+1)
		case "enter":
			return m.chooseOption()
		}
	case "answer":
		if key.String() == "o" && d.field == 0 {
			return m, openArtifacts(m.executionByID(d.request.ExecutionID), d.artifacts)
		}
		if key.String() == "tab" || key.String() == "shift+tab" {
			d.field = 1 - d.field
			return m, nil
		}
		if d.field == 0 {
			switch key.String() {
			case "up", "k":
				d.selected = max(0, d.selected-1)
			case "down", "j":
				if d.selected == len(d.options)-1 {
					d.field = 1
				} else {
					d.selected++
				}
			case "enter":
				d.field = 1
			}
			return m, nil
		}
		if key.Type == tea.KeyUp {
			d.field = 0
			return m, nil
		}
		if key.String() == "enter" {
			if len(d.options) == 0 {
				return m, nil
			}
			d.kind = "answer-confirm"
			return m, nil
		}
		command := d.updateInput(key)
		return m, command
	case "new", "retry", "message":
		if key.Type == tea.KeyDown || key.Type == tea.KeyTab {
			d.field = (d.field + 1) % (len(d.fields) + 1)
			d.scroll = 0
			return m, nil
		}
		if key.Type == tea.KeyUp || key.Type == tea.KeyShiftTab {
			d.field = (d.field + len(d.fields)) % (len(d.fields) + 1)
			d.scroll = 0
			return m, nil
		}
		if key.String() == "enter" {
			if d.field < len(d.fields) {
				d.field++
				return m, nil
			}
			if d.kind == "retry" {
				if strings.TrimSpace(d.fields[0].input.Value()) == "" {
					d.err = "Explain what changed before retrying."
					return m, nil
				}
				d.kind = "retry-confirm"
				return m, nil
			}
			return m.submitDialog()
		}
		command := d.updateInput(key)
		return m, command
	}
	return m, nil
}

func (m model) dialogScrollLimit() int {
	d := m.dialog
	width := min(max(48, m.width-8), 88) - 6
	lines := len(d.options)
	if d.detail != "" {
		lines += len(d.contentLines(width)) + 1
	}
	return max(0, lines-max(1, m.height-9))
}

func (d *dialog) updateInput(message tea.Msg) tea.Cmd {
	index := d.field
	switch d.kind {
	case "answer":
		if index != 1 {
			return nil
		}
		index = 0
	case "new", "retry", "message":
	default:
		return nil
	}
	if index >= len(d.fields) {
		return nil
	}
	entry := &d.fields[index]
	entry.input.Focus()
	input, command := entry.input.Update(message)
	if input.Value() != entry.input.Value() {
		d.err = ""
	}
	entry.input = input
	return command
}

func (m model) chooseOption() (tea.Model, tea.Cmd) {
	d := m.dialog
	switch d.kind {
	case "actions":
		if len(d.options) == 0 {
			return m, nil
		}
		switch d.options[d.selected] {
		case "New execution":
			m.dialog = m.newDialog()
		case "Inbox":
			m.dialog = nil
			m.focusInbox()
		case "Select context":
			m.dialog = m.contextDialog()
		case "Answer decision":
			m.dialog = m.answerDialog(m.pendingRequest())
		case "Retry blocked operation":
			m.dialog = m.retryDialog()
		case "Inspect hook":
			hook := m.retryHook()
			if hook != nil {
				m.dialog = &dialog{kind: "inspect", title: "Hook · " + hook.Context.Name, detail: fmt.Sprintf("%s/%s\nID %s\nState %s · try %d\nSession %s\n%s", hook.Context.Scope, hook.Context.Boundary, hook.ID, hook.State, hook.Try, hook.SessionID, hook.Error)}
			}
		case "Message agent":
			m.dialog = m.messageDialog()
		case "Stop agent session":
			m.dialog = m.stopAgentDialog()
		case "Inspect step":
			m.dialog = m.inspectDialog()
		case "Cancel execution":
			m.dialog = m.cancelDialog()
		}
	case "contexts":
		id := "*"
		if d.selected > 0 && d.selected <= len(d.contexts) {
			id = d.contexts[d.selected-1].ID
		}
		m.dialog = nil
		client := m.controller
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := client.SetContext(ctx, id)
			return action{message: "Context set to " + id, err: err}
		}
	case "agents":
		if d.selected < len(d.sessions) {
			m.showPage(logsPage)
			m.selectedSessionID = d.sessions[d.selected].ID
			m.output, m.outputID = "", ""
			m.focus, m.tab, m.dialog = 2, 1, nil
			m.logCache = nil
		}
	case "cancel", "stop-agent":
		if d.selected == 1 {
			return m.submitDialog()
		}
		m.dialog = nil
	}
	return m, nil
}

func (m model) submitDialog() (tea.Model, tea.Cmd) {
	d := m.dialog
	if d == nil {
		return m, nil
	}
	switch d.kind {
	case "new":
		request, err := buildExecRequest(d)
		if err != nil {
			m.dialog = errorDialog("Cannot start execution", err.Error(), d)
			return m, nil
		}
		m.dialog = nil
		m.notice = "Starting execution…"
		client := m.controller
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			record, err := client.Exec(ctx, request)
			message := "Execution " + shortID(record.ID) + " started"
			if err == nil && record.State == execution.ExecutionFailed {
				err = fmt.Errorf("execution %s failed to start: %s", record.ID, record.Error)
			}
			return action{message: message, executionID: record.ID, dialog: d, err: err}
		}
	case "answer-confirm":
		requestID, outcome, reason := d.request.ID, execution.Outcome(d.options[d.selected]), strings.TrimSpace(d.fields[0].input.Value())
		if d.request.Kind == "step_input" && len(d.request.Options) == 0 && reason == "" {
			m.dialog = errorDialog("Answer required", "Enter a response to the agent's question.", d)
			return m, nil
		}
		m.dialog = nil
		client := m.controller
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			record, err := client.Answer(ctx, requestID, outcome, reason, operator())
			return action{message: "Response recorded: " + string(outcome), executionID: record.ID, dialog: d, err: err}
		}
	case "retry-confirm":
		stepID, reason := d.targetID, strings.TrimSpace(d.fields[0].input.Value())
		m.dialog = nil
		client := m.controller
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			record, err := client.Retry(ctx, stepID, reason, operator())
			return action{message: "Step retry requested", executionID: record.ID, dialog: d, err: err}
		}
	case "cancel":
		id := d.targetID
		m.dialog = nil
		client := m.controller
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err := client.Cancel(ctx, id)
			return action{message: "Execution cancelled", executionID: id, dialog: d, err: err}
		}
	case "stop-agent":
		id := d.targetID
		m.dialog = nil
		client := m.supervisor
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := client.Stop(ctx, id)
			return action{message: "Agent stop requested", dialog: d, err: err}
		}
	case "message":
		body := strings.TrimSpace(d.fields[0].input.Value())
		path := strings.TrimSpace(d.fields[1].input.Value())
		if body != "" && path != "" {
			d.err = "Use either a message or a file."
			return m, nil
		}
		if path != "" {
			var err error
			path, err = expandHome(path)
			if err != nil {
				d.err = err.Error()
				return m, nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				d.err = err.Error()
				return m, nil
			}
			body = string(content)
		}
		if len(body) == 0 || len(body) > 16384 {
			d.err = "Message must contain 1–16384 bytes."
			return m, nil
		}
		id := d.targetID
		m.dialog = nil
		client := m.supervisor
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := client.Send(ctx, id, body)
			return action{message: "Message sent", dialog: d, err: err}
		}
	}
	return m, nil
}

func buildExecRequest(d *dialog) (control.ExecRequest, error) {
	brief := strings.TrimSpace(d.fields[0].input.Value())
	config := strings.TrimSpace(d.fields[2].input.Value())
	if config == "" {
		return control.ExecRequest{}, errors.New("Choose a factory.json file.")
	}
	config, err := expandHome(config)
	if err != nil {
		return control.ExecRequest{}, err
	}
	config, err = filepath.Abs(config)
	if err != nil {
		return control.ExecRequest{}, err
	}
	if info, err := os.Stat(config); err != nil || !info.Mode().IsRegular() {
		return control.ExecRequest{}, fmt.Errorf("config is not a file: %s", config)
	}
	var files []string
	for _, item := range strings.Split(d.fields[1].input.Value(), ",") {
		path := strings.TrimSpace(item)
		if path == "" {
			continue
		}
		path, err = expandHome(path)
		if err != nil {
			return control.ExecRequest{}, err
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return control.ExecRequest{}, err
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return control.ExecRequest{}, fmt.Errorf("input is not a file: %s", path)
		}
		files = append(files, path)
	}
	if brief == "" && len(files) == 0 {
		return control.ExecRequest{}, errors.New("Enter a brief or at least one input file.")
	}
	return control.ExecRequest{ConfigPath: config, Brief: brief, Files: files, Actor: operator()}, nil
}

func operator() string {
	if name := os.Getenv("USER"); name != "" {
		return name
	}
	return "tui"
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func reviewArtifacts(current *control.Execution, request *control.Request) []string {
	if current == nil {
		return nil
	}
	var decisionKey execution.StepID
	for _, visit := range current.Steps {
		if visit.ID == request.StepID {
			decisionKey = visit.Key
			break
		}
	}
	var names []string
	for _, step := range current.Configuration.Workflow.Steps {
		if step.ID == decisionKey && step.Decision != nil && len(step.Decision.Artifacts) > 0 {
			names = append(names, step.Decision.Artifacts...)
			break
		}
	}
	for _, step := range current.Configuration.Workflow.Steps {
		for _, visit := range current.Steps {
			if visit.Key == step.ID && visit.State == "completed" {
				names = append(names, step.Outputs...)
				break
			}
		}
	}
	seen := make(map[string]bool)
	var unique []string
	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}
	return unique
}

func openArtifacts(current *control.Execution, names []string) tea.Cmd {
	return func() tea.Msg {
		if current == nil || len(names) == 0 {
			return action{err: errors.New("No files are listed for this item.")}
		}
		for _, name := range names {
			path, err := executionFile(executionFilesRoot(current), name)
			if err != nil {
				return action{err: err}
			}
			var command *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				command = exec.Command("open", path)
			case "linux":
				command = exec.Command("xdg-open", path)
			case "windows":
				command = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
			default:
				return action{err: errors.New("No default file opener is available on this OS.")}
			}
			if err := command.Start(); err != nil {
				return action{err: fmt.Errorf("open %s: %w", name, err)}
			}
			_ = command.Process.Release()
		}
		return action{message: "Opened listed files"}
	}
}

func copyReviewFiles(project string, names []string) tea.Cmd {
	return func() tea.Msg {
		paths, err := reviewLocations(project, names)
		if err != nil {
			return action{err: err}
		}
		if err := clipboard.WriteAll(strings.Join(paths, "\n")); err != nil {
			return action{err: fmt.Errorf("copy file locations: %w", err)}
		}
		return action{message: "Copied file locations"}
	}
}

func executionFilesRoot(current *control.Execution) string {
	if current == nil {
		return ""
	}
	if current.Configuration.FilesDir != "" {
		return current.Configuration.FilesDir
	}
	return current.Project
}

func executionFile(project, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("file must be relative to execution storage: %s", name)
	}
	project, err := filepath.EvalSymlinks(project)
	if err != nil {
		return "", err
	}
	path := filepath.Join(project, name)
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("file %s: %w", name, err)
	}
	relative, err := filepath.Rel(project, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file leaves execution storage: %s", name)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("file %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("file is not regular: %s", name)
	}
	return path, nil
}

func (m model) dialogView() string {
	d := m.dialog
	if d == nil {
		return ""
	}
	width := min(max(48, m.width-8), 88)
	if m.width < 52 || m.height < 14 {
		return "Factory needs a terminal at least 52 × 14 for this dialog. Press Esc.\n"
	}
	inner := width - 6
	var lines []string
	focusLine := 0
	if d.detail != "" {
		lines = append(lines, d.contentLines(inner)...)
		lines = append(lines, "")
	}
	if len(d.artifacts) > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(accent).Render("Files · o open · c copy locations"))
		for _, name := range d.artifacts {
			lines = append(lines, "  "+name)
		}
		lines = append(lines, "")
	}
	for i, option := range d.options {
		marker := "  "
		if i == d.selected {
			marker = "▸ "
			focusLine = len(lines)
		}
		color := dim
		if i == d.selected {
			color = accent
		}
		lines = append(lines, selectedRow(marker+terminalLine(option), inner, i == d.selected, color))
	}
	if len(d.options) > 0 && len(d.fields) > 0 {
		lines = append(lines, "")
	}
	for i, entry := range d.fields {
		marker := "  "
		focused := i == d.field
		if d.kind == "answer" {
			focused = d.field == 1 && i == 0
		}
		if focused && d.kind != "answer-confirm" && d.kind != "retry-confirm" {
			marker = "▸ "
			if d.kind != "answer" || d.field == 1 {
				focusLine = len(lines)
			}
		}
		input := entry.input
		// The widget renders its cursor in addition to Width text columns.
		input.Width = max(1, inner-ansi.StringWidth(marker+entry.label+": ")-1)
		input.SetCursor(input.Position())
		background := raised
		if focused {
			background = selection
		}
		input.TextStyle = lipgloss.NewStyle().Foreground(bright).Background(background)
		input.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim).Background(background)
		if focused {
			input.Focus()
		} else {
			input.Blur()
		}
		fieldStyle := lipgloss.NewStyle().Foreground(bright).Background(raised)
		if focused {
			fieldStyle = fieldStyle.Background(selection)
		}
		lines = append(lines, fieldStyle.Width(inner).Render(marker+entry.label+": "+input.View()))
		for _, hint := range wrapLine("    "+entry.hint, inner) {
			lines = append(lines, lipgloss.NewStyle().Foreground(dim).Render(hint))
		}
	}
	if d.kind == "new" || d.kind == "message" || d.kind == "retry" {
		label := "Start execution"
		if d.kind == "message" {
			label = "Send message"
		}
		if d.kind == "retry" {
			label = "Retry step"
		}
		marker := "  "
		if d.field == len(d.fields) {
			marker = "▸ "
			focusLine = len(lines) + 1
		}
		lines = append(lines, "", lipgloss.NewStyle().Foreground(accent).Bold(true).Render(marker+"[ "+label+" ]"))
	}
	if d.err != "" {
		for _, line := range strings.Split(d.err, "\n") {
			for _, wrapped := range wrapLine(line, inner) {
				lines = append(lines, lipgloss.NewStyle().Foreground(bad).Render(wrapped))
			}
		}
	}
	if d.notice != "" {
		for _, line := range wrapLine(d.notice, inner) {
			lines = append(lines, tint(line, good, false))
		}
	}
	footer := "↑↓ choose · Enter select · Esc back"
	if d.rawDetail != "" {
		footer = "↑↓ scroll · v readable/raw · Enter/Esc close"
	}
	switch d.kind {
	case "new", "message":
		footer = "↑↓ fields / button · ←→ cursor · Enter select · Esc back"
	case "retry":
		footer = "Enter confirm retry · Esc back"
	case "answer":
		footer = "↑↓ outcome / reason · Enter confirm · Esc back"
	case "error":
		footer = "↑↓ or PgUp/PgDn scroll · Enter/Esc return to form"
	case "answer-confirm", "retry-confirm":
		footer = "y confirm · n back · Esc cancel"
	case "cancel", "stop-agent":
		footer = "Choose No or Yes, then Enter · Esc back"
	}
	maxLines := max(1, m.height-9)
	if len(lines) > maxLines {
		start := max(0, min(focusLine-maxLines/2+d.scroll, len(lines)-maxLines))
		if d.kind == "error" || d.kind == "inspect" || d.kind == "help" {
			start = max(0, min(d.scroll, len(lines)-maxLines))
		}
		lines = lines[start : start+maxLines]
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(warn).Render(ansi.Truncate(footer, inner, "…")))
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], inner, "…")
	}
	color := accent
	if d.kind == "error" {
		color = bad
	}
	content := badge(ansi.Truncate(terminalLine(strings.ToUpper(d.title)), max(1, inner-2), "…"), color) + "\n\n" + strings.Join(lines, "\n")
	box := lipgloss.NewStyle().Width(width-4).Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Background(surface).BorderBackground(canvas).Padding(1, 1).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box, lipgloss.WithWhitespaceBackground(canvas))
}

func wrapLine(line string, width int) []string {
	return strings.Split(ansi.Wrap(terminalText(line), max(1, width), ""), "\n")
}

func (m model) retryHook() *execution.HookRun {
	current := m.currentExecution()
	if current == nil || current.State != execution.ExecutionBlocked {
		return nil
	}
	for _, hook := range current.Hooks {
		if hook.ID == current.CurrentStepID && hook.State == "blocked" {
			return &hook
		}
	}
	return nil
}
