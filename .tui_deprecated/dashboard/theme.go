package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Shared surfaces and semantic colors keep selection separate from status.
var (
	canvas    = lipgloss.Color("#17191E")
	surface   = lipgloss.Color("#1D2026")
	raised    = lipgloss.Color("#292D35")
	selection = lipgloss.Color("#303B52")
)

func badge(label string, color lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(color).Background(raised).Bold(true).Padding(0, 1).Render(terminalLine(label))
}

func selectedRow(value string, width int, selected bool, color lipgloss.Color) string {
	style := lipgloss.NewStyle().Foreground(color).Background(surface).Width(max(1, width))
	if selected {
		style = style.Background(selection).Bold(true)
	}
	return style.Render(ansi.Truncate(value, max(1, width), "…"))
}

func fitRow(value string, width int) string {
	value = ansi.Truncate(value, max(1, width), "…")
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}
