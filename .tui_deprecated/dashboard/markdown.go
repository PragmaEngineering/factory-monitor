package dashboard

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Render only expanded content, with an explicit style (never auto-detection
// that can query the terminal). Cache by width and mode while the dialog lives.
func (d *dialog) contentLines(width int) []string {
	if d.renderWidth == width && d.renderRaw == d.raw && d.rendered != nil {
		return d.rendered
	}
	text := d.detail
	if d.raw && d.rawDetail != "" {
		text = d.rawDetail
	}
	text = terminalText(text)
	if d.markdown && !d.raw {
		style := styles.DarkStyleConfig
		zero := uint(0)
		style.Document.Margin = &zero
		renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(max(1, width)), glamour.WithColorProfile(lipgloss.ColorProfile()))
		if err == nil {
			if rendered, err := renderer.Render(text); err == nil {
				text = strings.TrimSpace(rendered)
			}
		}
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.Split(ansi.Hardwrap(line, max(1, width), true), "\n")...)
	}
	d.renderWidth, d.renderRaw, d.rendered = width, d.raw, lines
	return lines
}
