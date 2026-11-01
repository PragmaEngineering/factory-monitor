package dashboard

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type paneLayout struct {
	executions, center, inbox, body int
}

func layout(width, height int) paneLayout {
	left := max(18, min(30, width/6))
	right := max(24, min(48, width/4))
	return paneLayout{left, width - left - right - 2, right, height - 6}
}

func (m model) View() string {
	if m.dialog != nil {
		return m.dialogView()
	}
	if m.width < 72 || m.height < 18 {
		return "Factory monitor needs a terminal at least 72 × 18. Resize, or press q to quit.\n"
	}
	p := layout(m.width, m.height)
	var main string
	if m.fullscreen {
		main = m.panel(m.paneTitle(m.focus), m.paneLines(m.focus, m.width-4, p.body-3), m.width, p.body, true)
	} else if m.page == messagesPage {
		main = m.messagesView(m.width, p.body)
	} else if m.page == logsPage {
		main = m.panel(m.detailTitle(), m.detailLines(m.width-4, p.body-3), m.width, p.body, m.focus == 2)
	} else {
		panes := m.visiblePanes()
		left := m.panel(m.paneTitle(panes[0]), m.paneLines(panes[0], p.executions-4, p.body-3), p.executions, p.body, m.focus == panes[0])
		center := m.panel(m.paneTitle(panes[1]), m.paneLines(panes[1], p.center-4, p.body-3), p.center, p.body, m.focus == panes[1])
		right := m.panel(m.paneTitle(panes[2]), m.paneLines(panes[2], p.inbox-4, p.body-3), p.inbox, p.body, m.focus == panes[2])
		main = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", center, " ", right)
	}

	notice := m.notice
	if m.connectionError != "" {
		notice = "Connection failed: " + m.connectionError
	}
	view := m.header() + "\n" + main + "\n" +
		lipgloss.NewStyle().Foreground(warn).Render(ansi.Truncate(terminalLine(notice), m.width, "…")) + "\n" +
		m.mainMenu()
	return lipgloss.NewStyle().Background(canvas).Width(m.width).Render(view)
}

func (m model) header() string {
	width := m.width
	if width < 1 {
		width = 120
	}
	brand := lipgloss.NewStyle().Foreground(canvas).Background(accent).Bold(true).Padding(0, 1).Render("FACTORY")
	status, color := m.runningGlyph("running")+" CONNECTING", warn
	if m.connected {
		status, color = "● LIVE · service connected", good
	} else if m.connectionError != "" {
		status, color = "● DISCONNECTED", bad
	}
	right := badge(status, color)
	title := brand + "  " + tint("EXECUTION MONITOR", dim, false)
	first := title + strings.Repeat(" ", max(1, width-ansi.StringWidth(title)-ansi.StringWidth(right))) + right
	context := "No execution selected · n start a run"
	if current := m.currentExecution(); current != nil {
		context = fmt.Sprintf("%s  %s · context %s · %s", shortID(current.ID), terminalLine(current.Workflow), current.ContextID, current.State)
	}
	second := tint(ansi.Truncate(context, width, "…"), bright, false)
	return fitRow(first, width) + "\n" + fitRow(second, width)
}

// Panel dimensions include the frame. Lip Gloss's Width includes padding but
// excludes the border, so a width of width-4 would wrap the content twice and
// grow the panel vertically. Every row is fitted before the frame is applied.
func (m model) panel(title string, lines []string, width, height int, focused bool) string {
	color := border
	if focused {
		color = accent
	}
	contentWidth, contentHeight := max(1, width-4), max(1, height-2)
	rows := make([]string, contentHeight)
	titleColor := bright
	if focused {
		titleColor = accent
	}
	rows[0] = lipgloss.NewStyle().Foreground(titleColor).Background(raised).Bold(true).Width(contentWidth).Render(ansi.Truncate(terminalLine(title), contentWidth, "…"))
	for i := 1; i < contentHeight && i-1 < len(lines); i++ {
		line := strings.NewReplacer("\t", "    ", "\r", "", "\n", " ").Replace(lines[i-1])
		rows[i] = ansi.Truncate(line, contentWidth, "…")
	}
	return lipgloss.NewStyle().Width(width-2).Height(height-2).Padding(0, 1).
		Background(surface).Border(lipgloss.RoundedBorder()).BorderForeground(color).BorderBackground(canvas).
		MaxWidth(width).MaxHeight(height).Render(strings.Join(rows, "\n"))
}

func terminalText(value string) string {
	value = strings.ReplaceAll(ansi.Strip(value), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, value)
}

func terminalLine(value string) string { return strings.ReplaceAll(terminalText(value), "\n", " ") }
