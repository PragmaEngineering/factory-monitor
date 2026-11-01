package dashboard

import (
	"fmt"
	"sort"
	"time"

	"controls/internal/comms"
	"controls/internal/control"
	"controls/internal/sessions"

	"encoding/json"
	"github.com/a2aproject/a2a-go/a2a"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// An inbox contains recorded messages and workflow decision requests. It does
// not turn agent output or tool calls into invented chat messages.
type inboxItem struct {
	id, recipient, sender, body, state, preview, raw string
	at                                               time.Time
	request                                          *control.Request
	message                                          *sessions.Message
}

func decisionRecipient(current *control.Execution, request control.Request) string {
	if request.Kind == "step_input" {
		return "human"
	}
	for _, visit := range current.Steps {
		if visit.ID != request.StepID {
			continue
		}
		for _, step := range current.Configuration.Workflow.Steps {
			if step.ID == visit.Key && step.Decision != nil && step.Decision.Authority != "" {
				return terminalLine(step.Decision.Authority)
			}
		}
	}
	return "decision"
}

func (m model) inboxItems() []inboxItem {
	current := m.currentExecution()
	if current == nil {
		return nil
	}
	var items []inboxItem
	if m.inboxAgent == "" || m.inboxAgent == "@decisions" || m.inboxAgent == "@operator" {
		for _, request := range current.Requests {
			sender := "workflow"
			if request.Kind == "step_input" {
				sender = shortID(request.Owner)
				for _, message := range m.conversation {
					if message.Sender == request.Owner {
						sender = message.SenderName
						break
					}
				}
			}
			items = append(items, inboxItem{id: request.ID, recipient: decisionRecipient(current, request), sender: sender, body: request.Prompt,
				state: request.State, at: request.CreatedAt, request: &request})
		}
	}
	for _, message := range m.messages {
		var recipient, logicalRecipient string
		for _, agent := range m.sessions {
			if agent.ID == message.SessionID && agent.ExecutionID == current.ID {
				recipient = agent.Name
				for _, visit := range current.Steps {
					if visit.ID == agent.StepID {
						logicalRecipient = string(visit.Key)
					}
				}
				break
			}
		}
		if recipient == "" || (m.inboxAgent != "" && m.inboxAgent != recipient && m.inboxAgent != logicalRecipient) {
			continue
		}
		sender := message.Sender
		if sender == "" {
			sender = "operator"
		}
		items = append(items, inboxItem{id: message.ID, recipient: recipient, sender: sender, body: message.Body,
			state: string(message.State), at: message.CreatedAt, message: &message})
	}
	for _, message := range m.conversation {
		matches := m.page != messagesPage && m.matchesInboxAgent(message.SenderName)
		for _, recipient := range message.RecipientNames {
			matches = matches || m.matchesInboxAgent(recipient)
		}
		if !matches {
			continue
		}
		body, _ := json.MarshalIndent(message.Payload, "", "  ")
		recipient := strings.Join(message.RecipientNames, ",")
		if recipient == "" {
			recipient = "thread " + shortID(message.ThreadID)
		}
		items = append(items, inboxItem{id: message.ID, recipient: recipient, sender: message.SenderName, body: conversationBody(message.Payload), raw: string(body), preview: conversationPreview(message.Payload), state: message.Payload.Kind, at: message.At})
	}
	if m.page == homePage {
		operator := items[:0]
		for _, item := range items {
			if item.request != nil || operatorAudience(item.recipient) {
				operator = append(operator, item)
			}
		}
		items = operator
	}
	sort.SliceStable(items, func(i, j int) bool {
		pendingI, pendingJ := items[i].request != nil && items[i].state == "pending", items[j].request != nil && items[j].state == "pending"
		if pendingI != pendingJ {
			return pendingI
		}
		if items[i].at.Equal(items[j].at) {
			return items[i].id < items[j].id
		}
		return items[i].at.After(items[j].at)
	})
	return items
}

func (m model) inboxIndex(items []inboxItem) int {
	if m.inboxSelected == "" && len(items) > 0 {
		return 0
	}
	for i := range items {
		if items[i].id == m.inboxSelected {
			return i
		}
	}
	return -1
}

func (m model) selectedInboxItem() *inboxItem {
	items := m.inboxItems()
	index := m.inboxIndex(items)
	if index < 0 {
		return nil
	}
	return &items[index]
}

func (m model) inboxLines(width, height int) []string {
	filter := "all agents"
	if m.inboxAgent == "@decisions" {
		filter = "decisions"
	} else if m.inboxAgent != "" {
		filter = m.inboxAgent
	}
	hint := "g filter · " + filter + " · m send"
	if m.page == homePage {
		hint = "Operator requests and messages"
	}
	if m.page == messagesPage {
		hint = "Incoming messages · Enter inspect · m compose"
	}
	lines := []string{lipgloss.NewStyle().Foreground(dim).Render(ansi.Truncate(hint, width, "…"))}
	current := m.currentExecution()
	if current == nil {
		return append(lines, "", "Select an execution.")
	}
	items := m.inboxItems()
	if height <= 3 {
		if len(items) == 0 {
			return []string{"Inbox is empty."}
		}
		index := max(0, m.inboxIndex(items))
		item := items[index]
		preview := item.preview
		if preview == "" {
			preview = item.body
		}
		return []string{ansi.Truncate("▸ "+item.sender+": "+terminalLine(preview), width, "…")}
	}
	count := fmt.Sprintf("%s · %d items", shortID(current.ID), len(items))
	if len(m.conversation) >= 500 {
		count += " · latest 500 messages"
	}
	lines = append(lines, count, "")
	if len(items) == 0 {
		return append(lines, "No messages or decisions.")
	}
	index := m.inboxIndex(items)
	visible := max(1, (height-3)/3)
	start := max(0, min(index-visible/2, len(items)-visible))
	for i := start; i < min(len(items), start+visible); i++ {
		item := items[i]
		marker := "  "
		if i == index {
			marker = "▸ "
		}
		identity := fmt.Sprintf("%s:%s@%s:", item.recipient, item.sender, item.at.Local().Format("15:04:05"))
		color := bright
		if item.request != nil && item.state == "pending" {
			color = warn
		}
		lines = append(lines, selectedRow(marker+terminalLine(identity), width, i == index, color))
		preview := item.preview
		if preview == "" {
			preview = item.body
		}
		lines = append(lines, ansi.Truncate("  "+terminalLine(preview), width, "…"))
		state := item.state
		if item.request != nil {
			label := "decision"
			if item.request.Kind == "step_input" {
				label = "input"
			}
			state = label + " · " + state
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(dim).Render(ansi.Truncate("  "+state+" · Enter view", width, "…")))
	}
	return lines
}

func (m model) inboxItemDialog() *dialog {
	item := m.selectedInboxItem()
	if item == nil {
		return nil
	}
	if item.request != nil && item.request.State == "pending" {
		return m.answerDialog(item.request)
	}
	detail := fmt.Sprintf("To: %s\nFrom: %s\nTime: %s\nState: %s\n\n%s", item.recipient, item.sender,
		item.at.Local().Format(time.RFC3339), item.state, item.body)
	d := &dialog{kind: "inspect", title: "Inbox message", detail: detail, rawDetail: item.raw, markdown: true}
	if item.request != nil {
		d.detail += "\n\nAnswer: " + string(item.request.Answer) + "\nReason: " + item.request.Reason
		d.artifacts = reviewArtifacts(m.currentExecution(), item.request)
		d.filesRoot = executionFilesRoot(m.currentExecution())
	}
	return d
}

func (m model) agentSessionsDialog() *dialog {
	d := &dialog{kind: "agents", title: "Agent sessions", detail: "Select a session to inspect its output. Running sessions remain individually addressable."}
	if current := m.currentExecution(); current != nil {
		for _, session := range m.sessions {
			if session.ExecutionID != current.ID {
				continue
			}
			d.sessions = append(d.sessions, session)
			d.options = append(d.options, fmt.Sprintf("%s · %s · %s", session.Name, session.State, shortID(session.ID)))
			if session.ID == m.sessionID() {
				d.selected = len(d.sessions) - 1
			}
		}
	}
	if len(d.options) == 0 {
		d.options = []string{"No agent sessions for this execution yet."}
	}
	return d
}

func (m *model) focusInbox() {
	if m.page == logsPage {
		m.showPage(messagesPage)
	}
	if m.page == messagesPage {
		m.focus = 3
		return
	}
	m.focus, m.inboxAgent = 3, ""
	for _, item := range m.inboxItems() {
		if item.request != nil && item.state == "pending" {
			m.inboxSelected = item.id
			return
		}
	}
}

func (m model) inboxReviewFiles() (string, []string) {
	current, item := m.currentExecution(), m.selectedInboxItem()
	if current == nil || item == nil || item.request == nil {
		return "", nil
	}
	return executionFilesRoot(current), reviewArtifacts(current, item.request)
}

func (m model) runningAgents() int {
	count := 0
	for _, session := range m.sessions {
		if session.ExecutionID == m.executionID() && session.State == sessions.Running {
			count++
		}
	}
	return count
}

func reviewLocations(project string, names []string) ([]string, error) {
	if project == "" || len(names) == 0 {
		return nil, fmt.Errorf("no files are listed for this inbox item")
	}
	var paths []string
	for _, name := range names {
		path, err := executionFile(project, name)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func operatorAudience(recipients string) bool {
	for _, recipient := range strings.Split(recipients, ",") {
		if recipient == "human" || recipient == "operator" {
			return true
		}
	}
	return false
}

func (m model) matchesInboxAgent(name string) bool {
	if m.inboxAgent == "@operator" {
		return operatorAudience(name)
	}
	if m.inboxAgent == "" || m.inboxAgent == name {
		return true
	}
	if current := m.currentExecution(); current != nil {
		for _, step := range current.Configuration.Workflow.Steps {
			if string(step.ID) == name && step.Run != nil && step.Run.Package == m.inboxAgent {
				return true
			}
		}
	}
	return false
}

func conversationPreview(payload comms.Payload) string {
	// Retain the full envelope for Enter; show useful content in the list.
	for _, part := range payload.Parts {
		if text, ok := part.(a2a.TextPart); ok && text.Text != "" {
			return text.Text
		}
	}
	for _, part := range payload.Parts {
		if data, ok := part.(a2a.DataPart); ok {
			if summary, ok := data.Data["summary"].(string); ok {
				return summary
			}
			if prompt, ok := data.Data["prompt"].(string); ok {
				return prompt
			}
		}
	}
	return payload.Kind
}

func conversationBody(payload comms.Payload) string {
	var sections []string
	for _, part := range payload.Parts {
		if text, ok := part.(a2a.TextPart); ok {
			sections = append(sections, text.Text)
			continue
		}
		raw, err := json.MarshalIndent(part, "", "  ")
		if err == nil {
			sections = append(sections, "```json\n"+string(raw)+"\n```")
		}
	}
	return strings.Join(sections, "\n\n")
}
