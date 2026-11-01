package dashboard

import (
	"controls/internal/sessions"
	"controls/internal/telemetry"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkflowResourceMetricsStayInsideTheView(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := workflowExample()
	now := time.Now()
	progress := now.Add(-35 * time.Second)
	m.sessions = []sessions.Session{{ID: "agent-build", ExecutionID: "preview-run", StepID: "build-visit", Name: "builder", State: sessions.Running, StartedAt: now.Add(-time.Minute), Resources: telemetry.Summary{Events: 8, Samples: 2, Tokens: telemetry.Tokens{Input: 3000, CacheRead: 42000, CacheWrite: 100, Output: 900}, ToolFailures: 1, LastProgressAt: &progress, Active: map[string]telemetry.Tool{"tool": {Command: "go test -race ./... " + strings.Repeat("long", 1000), StartedAt: progress}}}}}
	m.executions[0].Steps[3].Sessions = []string{"agent-build"}
	for _, size := range [][2]int{{72, 18}, {120, 36}, {180, 48}, {218, 70}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		require.LessOrEqual(t, lipgloss.Width(view), size[0])
		require.LessOrEqual(t, lipgloss.Height(view), size[1])
	}
	card := ansi.Strip(m.stepCard(m.executions[0].Configuration.Workflow.Steps[3], 55, true, now))
	require.Contains(t, card, "cache R/W")
	require.Contains(t, card, "Running:")
	require.Contains(t, card, "Since progress")
	require.Contains(t, card, "cost unknown")
	m.width, m.height = 180, 48
	if destination := os.Getenv("FACTORY_MONITOR_PREVIEW"); destination != "" {
		require.NoError(t, os.MkdirAll(destination, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(destination, "resources.ansi"), []byte(m.View()), 0644))
	}
}
