package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controls/execution"
	"controls/internal/control"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func pressKey(m *model, key tea.KeyMsg) tea.Cmd {
	updated, command := m.Update(key)
	*m = updated.(model)
	return command
}

func typeText(m *model, text string) {
	for _, char := range text {
		kind := tea.KeyRunes
		if char == ' ' {
			kind = tea.KeySpace
		}
		pressKey(m, tea.KeyMsg{Type: kind, Runes: []rune{char}})
	}
}

func TestNewExecutionUsingRealSpaceAndArrowKeyEvents(t *testing.T) {
	config := filepath.Join(t.TempDir(), "factory.json")
	require.NoError(t, os.WriteFile(config, []byte("{}"), 0600))
	want := "Create a minimal Go hello world"
	called := false
	client := testClient(t, func(request *http.Request) {
		called = true
		var body control.ExecRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		require.Equal(t, want, body.Brief)
		require.Equal(t, config, body.ConfigPath)
	})
	m := model{controller: client, configPath: config, width: 80, height: 24}
	pressKey(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	typeText(&m, want)
	require.Equal(t, want, m.dialog.fields[0].input.Value())
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "Brief:") {
			require.Contains(t, line, want)
			require.NotContains(t, line, "…")
		}
	}
	// Left/right edit text rather than switching focus.
	pressKey(&m, tea.KeyMsg{Type: tea.KeyLeft})
	typeText(&m, "!")
	pressKey(&m, tea.KeyMsg{Type: tea.KeyBackspace})
	pressKey(&m, tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, want, m.dialog.fields[0].input.Value())
	for i := 0; i < 3; i++ {
		pressKey(&m, tea.KeyMsg{Type: tea.KeyDown})
	}
	require.Equal(t, 3, m.dialog.field)
	require.Contains(t, ansi.Strip(m.View()), "▸ [ Start execution ]")
	pressKey(&m, tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 2, m.dialog.field)
	pressKey(&m, tea.KeyMsg{Type: tea.KeyDown})
	command := pressKey(&m, tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, command)
	require.NoError(t, command().(action).err)
	require.True(t, called)
}

func TestSubmissionErrorCanBeReadAndReturnedFromWithoutLosingInput(t *testing.T) {
	config := filepath.Join(t.TempDir(), "factory.json")
	require.NoError(t, os.WriteFile(config, []byte("{}"), 0600))
	problem := strings.Repeat("An important diagnostic line that must remain readable.\n", 35) + "FINAL CAUSE: agent directory does not exist"
	client := control.NewClientWithHTTPClient(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		body, err := json.Marshal(map[string]string{"error": problem})
		require.NoError(t, err)
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})})
	m := model{controller: client, configPath: config, width: 80, height: 24, refreshing: true}
	pressKey(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	typeText(&m, "Build hello world")
	for i := 0; i < 3; i++ {
		pressKey(&m, tea.KeyMsg{Type: tea.KeyDown})
	}
	command := pressKey(&m, tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, command)
	updated, _ := m.Update(command())
	m = updated.(model)
	require.Equal(t, "error", m.dialog.kind)
	require.Equal(t, problem, m.dialog.detail)
	for _, height := range []int{18, 24, 40} {
		m.height = height
		pressKey(&m, tea.KeyMsg{Type: tea.KeyHome})
		require.Contains(t, ansi.Strip(m.View()), "An important diagnostic")
		pressKey(&m, tea.KeyMsg{Type: tea.KeyEnd})
		require.Contains(t, ansi.Strip(m.View()), "FINAL CAUSE: agent directory does not exist")
		require.LessOrEqual(t, lipgloss.Width(m.View()), m.width)
		require.LessOrEqual(t, lipgloss.Height(m.View()), m.height)
	}
	pressKey(&m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, "new", m.dialog.kind)
	require.Equal(t, "Build hello world", m.dialog.fields[0].input.Value())
	require.Equal(t, config, m.dialog.fields[2].input.Value())
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func testClient(t *testing.T, check func(*http.Request)) *control.Client {
	t.Helper()
	return control.NewClientWithHTTPClient(&http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		check(request)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"run-1","state":"running"}`)), Header: make(http.Header)}, nil
	})})
}

func TestNewExecutionFormUsesProjectConfigAndInputFiles(t *testing.T) {
	project := t.TempDir()
	config := filepath.Join(project, "factory.json")
	input := filepath.Join(project, "request with spaces.md")
	require.NoError(t, os.WriteFile(config, []byte("{}"), 0600))
	require.NoError(t, os.WriteFile(input, []byte("details"), 0600))
	called := false
	client := testClient(t, func(request *http.Request) {
		called = true
		require.Equal(t, "POST", request.Method)
		require.Equal(t, "/executions", request.URL.Path)
		var body control.ExecRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		require.Equal(t, config, body.ConfigPath)
		require.Equal(t, "Make hello world", body.Brief)
		require.Equal(t, []string{input}, body.Files)
	})
	m := model{controller: client, configPath: config}
	m.dialog = m.newDialog()
	m.dialog.fields[0].input.SetValue("Make hello world")
	m.dialog.fields[1].input.SetValue(input)
	_, command := m.submitDialog()
	require.NotNil(t, command)
	result := command().(action)
	require.NoError(t, result.err)
	require.Equal(t, "run-1", result.executionID)
	require.True(t, called)
}

func TestDecisionAcceptsFeedbackAndConfirmsBeforeSend(t *testing.T) {
	called := false
	client := testClient(t, func(request *http.Request) {
		called = true
		require.Equal(t, "/requests/request-1/answer", request.URL.Path)
		var body struct {
			Outcome string `json:"outcome"`
			Reason  string `json:"reason"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		require.Equal(t, "changes_requested", body.Outcome)
		require.Equal(t, "Correct the API contract", body.Reason)
	})
	request := control.Request{ID: "request-1", Options: []execution.Outcome{"approve", "changes_requested"}}
	m := model{controller: client, dialog: &dialog{kind: "answer", request: &request,
		options: []string{"approve", "changes_requested"}, selected: 1, field: 1,
		fields: []field{inputField("Reason", "", "")}}}
	updated, command := m.dialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Correct the API contract")})
	require.Nil(t, command)
	m = updated.(model)
	require.Equal(t, "Correct the API contract", m.dialog.fields[0].input.Value())
	updated, command = m.dialogKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, command)
	m = updated.(model)
	require.Equal(t, "answer-confirm", m.dialog.kind)
	require.False(t, called)
	updated, command = m.dialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	require.NotNil(t, command)
	m = updated.(model)
	require.Nil(t, m.dialog)
	require.NoError(t, command().(action).err)
	require.True(t, called)
}

func TestReviewFilesStayInsideStorage(t *testing.T) {
	project := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.html")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(project, "spec.html"), []byte("spec"), 0600))
	_, err := executionFile(project, "spec.html")
	require.NoError(t, err)
	_, err = executionFile(project, "../outside.html")
	require.Error(t, err)
	require.NoError(t, os.Symlink(outside, filepath.Join(project, "link.html")))
	_, err = executionFile(project, "link.html")
	require.Error(t, err)
}

func TestDecisionShowsPinnedAndCompletedReviewFiles(t *testing.T) {
	record := &control.Execution{Configuration: execution.Configuration{Workflow: execution.Workflow{Steps: []execution.Step{
		{ID: "spec", Outputs: []string{"spec.html", "plan.html"}},
		{ID: "review", Outputs: []string{"reviews/architecture.md"}},
		{ID: "approve", Decision: &execution.DecisionSpec{Artifacts: []string{"spec.html", "plan.html"}}},
	}}}, Steps: []control.Step{{ID: "visit-1", Key: "spec", State: "completed"}, {ID: "visit-2", Key: "review", State: "completed"}, {ID: "visit-3", Key: "approve", State: "waiting_input"}}}
	files := reviewArtifacts(record, &control.Request{StepID: "visit-3", Prompt: "Review limit reached. Approve design"})
	require.Equal(t, []string{"spec.html", "plan.html", "reviews/architecture.md"}, files)
}

func TestRetryAndCancelUseSelectedResourceIDs(t *testing.T) {
	for _, scenario := range []struct{ kind, target, path string }{
		{"retry-confirm", "step-7", "/steps/step-7/retry"},
		{"cancel", "run-8", "/executions/run-8/cancel"},
	} {
		t.Run(scenario.kind, func(t *testing.T) {
			client := testClient(t, func(request *http.Request) { require.Equal(t, scenario.path, request.URL.Path) })
			m := model{controller: client, dialog: &dialog{kind: scenario.kind, targetID: scenario.target,
				fields: []field{inputField("Reason", "Provider recovered", "")}}}
			_, command := m.submitDialog()
			require.NotNil(t, command)
			require.NoError(t, command().(action).err)
		})
	}
}

func TestDialogFitsTerminalAndKeepsFocusedOptionVisible(t *testing.T) {
	d := &dialog{kind: "agents", title: "Agent sessions", selected: 25}
	for i := range 30 {
		d.options = append(d.options, "request "+string(rune('A'+i)))
	}
	m := model{dialog: d, width: 80, height: 20}
	view := m.View()
	require.Contains(t, view, "request Z")
	require.LessOrEqual(t, lipgloss.Width(view), m.width)
	require.LessOrEqual(t, lipgloss.Height(view), m.height)
}

func TestDecisionReviewUsesPinnedExecutionFiles(t *testing.T) {
	project := t.TempDir()
	files := t.TempDir()
	record := &control.Execution{Project: project, Configuration: execution.Configuration{FilesDir: files}}
	require.Equal(t, files, executionFilesRoot(record))
	require.NoError(t, os.WriteFile(filepath.Join(files, "spec.html"), []byte("execution spec"), 0600))
	path, err := executionFile(executionFilesRoot(record), "spec.html")
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(filepath.Join(files, "spec.html"))
	require.NoError(t, err)
	require.Equal(t, expected, path)
}
