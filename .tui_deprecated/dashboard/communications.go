package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type communicationEdge struct {
	from, to string
	count    int
	last     time.Time
}

// The map describes observed message traffic, never inferred workflow routes.
// Counts apply to the retained inbox window, not the execution's lifetime.
func (m model) communicationEdges() []communicationEdge {
	edges := make(map[[2]string]communicationEdge)
	add := func(from, to string, at time.Time) {
		if from == "" || to == "" {
			return
		}
		key := [2]string{from, to}
		edge := edges[key]
		edge.from, edge.to, edge.count = from, to, edge.count+1
		if at.After(edge.last) {
			edge.last = at
		}
		edges[key] = edge
	}
	for _, message := range m.conversation {
		for _, recipient := range message.RecipientNames {
			add(message.SenderName, recipient, message.At)
		}
	}
	for _, message := range m.messages {
		for _, session := range m.sessions {
			if session.ID != message.SessionID || session.ExecutionID != m.executionID() {
				continue
			}
			name := session.Name
			if current := m.currentExecution(); current != nil {
				for _, visit := range current.Steps {
					if visit.ID == session.StepID {
						name = string(visit.Key)
					}
				}
			}
			sender := message.Sender
			if sender == "" {
				sender = "operator"
			}
			add(sender, name, message.CreatedAt)
		}
	}
	var result []communicationEdge
	for _, edge := range edges {
		result = append(result, edge)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].last.Equal(result[j].last) {
			return result[i].last.After(result[j].last)
		}
		if result[i].from != result[j].from {
			return result[i].from < result[j].from
		}
		return result[i].to < result[j].to
	})
	return result
}

// Traffic lanes use recorded directed delivery counts. A recent packet is
// highlighted only when traffic actually occurred in the last five seconds.
func (m model) trafficLines(width, height int) []string {
	if m.currentExecution() == nil {
		return []string{"Select an execution on Home."}
	}
	edges := m.communicationEdges()
	total, maximum := 0, 1
	for _, edge := range edges {
		total += edge.count
		maximum = max(maximum, edge.count)
	}
	lines := []string{tint(ansi.Truncate(fmt.Sprintf("◈ %d deliveries  ·  %d directed links  ·  retained window", total, len(edges)), width, "…"), dim, false)}
	if len(edges) == 0 {
		return append(lines, "   ○ ┄┄┄┄┄┄┄┄▶ ○   Waiting for message traffic")
	}
	for _, edge := range edges {
		if len(lines) >= height {
			break
		}
		packet := "▶"
		color := accent
		if time.Since(edge.last) < 5*time.Second {
			packet, color = "●▶", accent
		}
		labelWidth := max(6, (width-28)/2)
		from := ansi.Truncate(edge.from, labelWidth, "…")
		to := ansi.Truncate(edge.to, labelWidth, "…")
		volume := strings.Repeat("▰", max(1, edge.count*6/maximum))
		lane := lipgloss.NewStyle().Foreground(bright).Background(raised).Render("["+from+"]") + tint(fmt.Sprintf(" ━━━ %d %s ", edge.count, packet), color, false) + lipgloss.NewStyle().Foreground(bright).Background(raised).Render("["+to+"]") + tint("  "+volume, color, false)
		lines = append(lines, ansi.Truncate(lane, width, "…"))
	}
	return lines
}
