package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Keep navigation and actions separate. Contextual actions belong in the
// selected item's dialog, not in the global menu.
func (m model) mainMenu() string {
	var tabs []string
	for _, tab := range []struct {
		label string
		page  monitorPage
	}{{"1 Home", homePage}, {"2 Messages", messagesPage}, {"3 Logs", logsPage}} {
		style := lipgloss.NewStyle().Foreground(dim).Background(canvas).Padding(0, 1)
		if m.page == tab.page {
			style = style.Foreground(bright).Background(selection).Bold(true)
		}
		tabs = append(tabs, style.Render(tab.label))
	}
	left := strings.Join(tabs, " ")
	zoom := "z Expand"
	if m.fullscreen {
		zoom = "z / Esc Restore"
	}
	right := lipgloss.NewStyle().Foreground(dim).Render(zoom + " ")
	navigation := left + strings.Repeat(" ", max(1, m.width-ansi.StringWidth(left)-ansi.StringWidth(right))) + right

	keyStyle := lipgloss.NewStyle().Foreground(accent).Background(canvas)
	labelStyle := lipgloss.NewStyle().Foreground(dim).Background(canvas)
	var actions []string
	for _, item := range []struct{ key, label string }{
		{"n", "New"}, {"a", "Actions"}, {"c", "Context"}, {"s", "Agents"},
		{"i", "Inbox"}, {"Tab", "Panes"}, {"?", "Help"}, {"q", "Quit"},
	} {
		actions = append(actions, keyStyle.Render(item.key)+labelStyle.Render(" "+item.label))
	}
	// One space at narrow widths keeps every action visible; wider terminals
	// get breathing room without turning each shortcut into a separate button.
	gap := " "
	if m.width >= 88 {
		gap = "   "
	}
	row := " " + strings.Join(actions, gap)
	return fitRow(navigation, m.width) + "\n" + fitRow(row, m.width)
}
