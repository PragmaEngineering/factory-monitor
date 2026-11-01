package dashboard

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"controls/internal/state"
	"controls/internal/telemetry"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Readable logs are a presentation of retained output, never a completion signal.
// Every record remains available through the raw view and entry inspection.
type logEntry struct {
	source, kind, body, raw string
	at                      time.Time
	failed                  bool
	records                 int
}

type runnerEvent struct {
	Type    string          `json:"type"`
	Tool    string          `json:"toolName"`
	Error   bool            `json:"isError"`
	Success *bool           `json:"success"`
	Failure json.RawMessage `json:"error"`
	Message struct {
		Role       string `json:"role"`
		Error      string `json:"errorMessage"`
		StopReason string `json:"stopReason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
	Result json.RawMessage `json:"result"`
	Args   json.RawMessage `json:"args"`
}

func contentText(raw json.RawMessage) string {
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &result) == nil {
		var parts []string
		for _, part := range result.Content {
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return string(raw)
}

func parseLogRecord(source, line string, at time.Time) logEntry {
	entry := logEntry{source: source, kind: "OUTPUT", body: line, raw: line, at: at, records: 1}
	var event runnerEvent
	if json.Unmarshal([]byte(line), &event) != nil || event.Type == "" {
		return entry
	}
	entry.failed = event.Error || (event.Success != nil && !*event.Success)
	switch event.Type {
	case telemetry.Protocol:
		var metric telemetry.Event
		_ = json.Unmarshal([]byte(line), &metric)
		entry.kind = "RESOURCE"
		entry.body = metric.Kind
		if metric.Kind == "usage" {
			s := telemetry.Summary{Tokens: metric.Tokens, Samples: 1}
			if metric.CostUSD != nil {
				s.CostSamples = 1
				s.CostUSD = *metric.CostUSD
			}
			entry.body = s.Usage() + " · cost " + s.Cost()
		}
	case "tool_execution_start":
		entry.kind = "TOOL"
		entry.body = event.Tool + "\n" + string(event.Args)
	case "tool_execution_end":
		entry.kind = "RESULT"
		entry.body = event.Tool + "\n" + contentText(event.Result)
		entry.failed = event.Error
	case "message_end":
		entry.kind = "RESPONSE"
		if event.Message.Role != "assistant" {
			entry.kind = "PROTOCOL"
			entry.body = event.Type + " · " + event.Message.Role
			break
		}
		var text []string
		for _, part := range event.Message.Content {
			if part.Type == "text" {
				text = append(text, part.Text)
			}
		}
		entry.body = strings.Join(text, "\n")
		if entry.body == "" {
			entry.kind = "PROTOCOL"
			entry.body = "Assistant turn · no text output"
		}
		if event.Message.Error != "" {
			entry.body = event.Message.Error
			entry.failed = true
		}
		if event.Message.StopReason == "error" || event.Message.StopReason == "aborted" {
			entry.failed = true
			entry.kind = "ERROR"
		}
	case "error":
		entry.kind = "ERROR"
		entry.failed = true
	case "agent_settled":
		entry.kind = "SESSION"
		entry.body = "Model turn settled · assignment status is recorded separately"
	case "agent_start", "agent_end", "turn_start", "turn_end", "message_start", "message_update", "tool_execution_update", "response":
		entry.kind = "PROTOCOL"
		entry.body = event.Type
	default:
		entry.kind = "EVENT · " + event.Type // Unknown runner events stay visible.
	}
	if entry.failed && entry.kind == "PROTOCOL" {
		entry.kind = "ERROR"
		var reason string
		if json.Unmarshal(event.Failure, &reason) != nil {
			reason = string(event.Failure)
		}
		entry.body = event.Type + " failed: " + reason
		if reason == "" {
			entry.body = line
		}
	}
	return entry
}

func readableLogs(activity []state.Activity) []logEntry {
	var entries []logEntry
	pending := make(map[string]string)
	order := []string{}
	last := make(map[string]time.Time)
	add := func(entry logEntry) {
		// Consecutive native streaming/lifecycle records share a compact row. The
		// exact records remain attached, so inspection and raw mode lose no data.
		if entry.kind == "PROTOCOL" && len(entries) > 0 {
			previous := &entries[len(entries)-1]
			if previous.source == entry.source && previous.kind == entry.kind {
				previous.raw += "\n" + entry.raw
				previous.records++
				previous.body = fmt.Sprintf("%d runner updates · latest %s", previous.records, entry.body)
				return
			}
		}
		entries = append(entries, entry)
	}
	for _, item := range activity {
		if _, ok := pending[item.Source]; !ok {
			order = append(order, item.Source)
		}
		pending[item.Source] += string(item.Data)
		last[item.Source] = item.At
		for {
			line, rest, complete := strings.Cut(pending[item.Source], "\n")
			if !complete {
				break
			}
			pending[item.Source] = rest
			if line != "" {
				add(parseLogRecord(item.Source, strings.TrimSuffix(line, "\r"), item.At))
			}
		}
	}
	for _, source := range order {
		if pending[source] != "" {
			add(parseLogRecord(source, pending[source], last[source]))
		}
	}
	return entries
}

func (m model) logRecords() []logEntry {
	if m.logCache != nil {
		return m.logCache
	}
	switch m.tab {
	case 0:
		return readableLogs(m.activity)
	case 1:
		if m.outputID != "" && m.outputID == m.sessionID() {
			return readableLogs([]state.Activity{{Source: "agent/" + m.outputID, Data: []byte(m.output)}})
		}
	case 2:
		if current := m.currentExecution(); current != nil {
			var entries []logEntry
			for _, event := range current.Events {
				entries = append(entries, logEntry{source: "workflow", kind: event.Kind, body: event.Message, raw: event.Message, at: event.At, records: 1, failed: strings.Contains(event.Kind, "failed")})
			}
			return entries
		}
	}
	return nil
}

func (m *model) rebuildLogs() {
	var selected *logEntry
	if !m.rawLogs && !m.follow && len(m.logCache) > 0 {
		entry := m.logCache[max(0, len(m.logCache)-1-m.scroll)]
		selected = &entry
	}
	m.logCache = nil
	if m.page == logsPage {
		m.logCache = m.logRecords()
		if selected != nil {
			for i, entry := range m.logCache {
				if entry.source == selected.source && entry.kind == selected.kind && entry.at.Equal(selected.at) && strings.HasPrefix(entry.raw, selected.raw) {
					m.scroll = len(m.logCache) - 1 - i
					break
				}
			}
		}
	}
}

func (m model) logEntryDialog() *dialog {
	entries := m.logRecords()
	if len(entries) == 0 {
		return nil
	}
	entry := entries[max(0, len(entries)-1-m.scroll)]
	return &dialog{kind: "inspect", title: entry.kind + " · " + entry.source, detail: entry.body, rawDetail: entry.raw, markdown: entry.kind == "RESPONSE"}
}

func (m model) detailLines(width, height int) []string {
	var heading []string
	if agent := m.activeSession(); m.tab == 1 && agent != nil && agent.Runner != "hook" {
		end := time.Now()
		if agent.EndedAt != nil {
			end = *agent.EndedAt
		}
		progress := agent.StartedAt
		if agent.Resources.LastProgressAt != nil {
			progress = *agent.Resources.LastProgressAt
		}
		heading = append(heading, wrapWorkflowText(agent.Resources.Usage(), width, accent)...)
		heading = append(heading, wrapWorkflowText(fmt.Sprintf("Elapsed %s · since progress %s · tool failures %s · cost %s", duration(end.Sub(agent.StartedAt)), duration(end.Sub(progress)), agent.Resources.Failures(), agent.Resources.Cost()), width, dim)...)
		if command := agent.Resources.Command(); command != "" && agent.State == "running" {
			heading = append(heading, wrapWorkflowText("Running: "+ansi.Truncate(terminalLine(command), max(1, width*2-9), "…"), width, bright)...)
		}
		if len(agent.BudgetNotices) > 0 {
			heading = append(heading, wrapWorkflowText(agent.BudgetNotices[len(agent.BudgetNotices)-1].Message, width, warn)...)
		}
	}
	if len(heading) >= height {
		return heading[:max(0, height)]
	}
	return append(heading, m.logDetailLines(width, height-len(heading))...)
}
func (m model) logDetailLines(width, height int) []string {
	if m.rawLogs {
		return m.rawDetailLines(width, height)
	}
	entries := m.logRecords()
	if len(entries) == 0 {
		return []string{tint("No activity yet.", dim, false)}
	}
	selected := max(0, len(entries)-1-m.scroll)
	var rows []string
	for i := selected; i >= 0; i-- {
		entry := entries[i]
		color := dim
		if entry.kind == "RESPONSE" {
			color = bright
		} else if entry.kind == "TOOL" {
			color = accent
		}
		if entry.failed {
			color = bad
		}
		stamp := "—"
		if !entry.at.IsZero() {
			stamp = entry.at.Local().Format("15:04:05")
		}
		prefix := "  "
		if i == selected {
			prefix = "▸ "
		}
		heading := fmt.Sprintf("%s%s  %s  %s", prefix, stamp, entry.kind, entry.source)
		row := []string{selectedRow(heading, width, i == selected, color)}
		preview := ansi.Truncate(terminalLine(entry.body), max(1, width-2), "…")
		row = append(row, tint("  "+preview, bright, false), "")
		if len(rows)+len(row) > height && len(rows) > 0 {
			break
		}
		rows = append(row, rows...)
	}
	view := viewport.New(max(1, width), max(1, height))
	view.Style = lipgloss.NewStyle().Background(surface).Foreground(bright)
	view.SetContent(strings.Join(rows, "\n"))
	return strings.Split(view.View(), "\n")
}

func (m model) rawDetailLines(width, height int) []string {
	var lines []string
	switch m.tab {
	case 0:
		for _, item := range m.activity {
			lines = append(lines, activityLines(item, width)...)
		}
	case 1:
		if m.outputID != "" && m.outputID == m.sessionID() {
			lines = strings.Split(terminalText(m.output), "\n")
		}
	case 2:
		if current := m.currentExecution(); current != nil {
			for _, event := range current.Events {
				lines = append(lines, fmt.Sprintf("%s  %s  %s", event.At.Local().Format("15:04:05"), event.Kind, event.Message))
			}
		}
	}
	if len(lines) == 0 {
		return []string{"No activity yet."}
	}
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(line, max(1, width), true), "\n")...)
	}
	end := max(1, len(wrapped)-m.scroll)
	start := max(0, end-max(1, height))
	return wrapped[start:end]
}
