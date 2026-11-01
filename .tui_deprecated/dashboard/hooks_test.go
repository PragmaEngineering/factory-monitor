package dashboard

import (
	"strings"
	"testing"

	"controls/execution"

	"github.com/stretchr/testify/require"
)

func TestBlockedHookAppearsInStatusAndExistingActions(t *testing.T) {
	m := workflowExample()
	record := &m.executions[0]
	record.State = execution.ExecutionBlocked
	record.CurrentStepID = "hook-id"
	record.Hooks = []execution.HookRun{{ID: "hook-id", Context: execution.HookContext{Name: "commit-work", Scope: "stage", Boundary: "after"}, State: "blocked", Error: "signing unavailable", Try: 1}}
	dialog := m.actionsDialog()
	require.Contains(t, dialog.options, "Retry blocked operation")
	require.Contains(t, dialog.options, "Inspect hook")
	retry := m.retryDialog()
	require.Equal(t, "hook-id", retry.targetID)
	require.Contains(t, retry.detail, "signing unavailable")
	view := strings.Join(m.workflowSummary(140, record.CreatedAt), "\n")
	require.Contains(t, view, "commit-work")
	require.Contains(t, view, "blocked")
}
