package dashboard

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controls/internal/sessions"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func messageExample() model {
	m := inboxExample()
	m.width, m.height = 120, 36
	m.showPage(messagesPage)
	m.inboxAgent = "build"
	m.sessions[0].StepID = "build-visit"
	m.loadComposer()
	return m
}
func keypress(m model, key tea.KeyMsg) (model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(model), cmd
}

func TestComposerTypingDoesNotTriggerNavigation(t *testing.T) {
	m := messageExample()
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	require.Equal(t, 4, m.focus)
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello a q 123 z world")})
	require.Equal(t, "hello a q 123 z world", m.composer.Value())
	require.Equal(t, messagesPage, m.page)
	require.Nil(t, m.dialog)
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, 3, m.focus)
	require.Equal(t, "hello a q 123 z world", m.composerDrafts[m.composerDraftKey])
	m, _ = keypress(m, tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 4, m.focus)
	require.True(t, m.composer.Focused(), "Tab must focus the actual input, not only its border")
}

func TestRecipientIndexRetainsSeparateDrafts(t *testing.T) {
	m := messageExample()
	m.composer.SetValue("builder draft")
	require.True(t, m.moveRecipient(1))
	require.Equal(t, "code_review", m.inboxAgent)
	require.Empty(t, m.composer.Value())
	m.composer.SetValue("reviewer draft")
	require.True(t, m.moveRecipient(-1))
	require.Equal(t, "builder draft", m.composer.Value())
	m.showPage(logsPage)
	m.showPage(messagesPage)
	require.Equal(t, "@operator", m.inboxAgent)
	_, cmd := m.sendComposer()
	require.Nil(t, cmd, "operator inbox has no recipient session")
}

func TestInlineSendPinsRecipientAndRetainsDraftOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			root, err := os.MkdirTemp("/private/tmp", "factory-compose-")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(root) })
			socket := filepath.Join(root, "service.sock")
			listener, err := net.Listen("unix", socket)
			require.NoError(t, err)
			var path, body string
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path = r.URL.Path
				var input map[string]string
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				body = input["body"]
				w.Header().Set("Content-Type", "application/json")
				if fail {
					w.WriteHeader(500)
					_, _ = w.Write([]byte(`{"error":"delivery unavailable"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(sessions.Message{ID: "new", SessionID: "coder", Body: body})
			})}
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(func() { _ = server.Close() })
			m := messageExample()
			m.supervisor = sessions.NewClient(socket)
			m.selectedSessionID = "reviewer" // must not redirect a message composed for build
			m.composer.SetValue("Please check the edge cases.")
			next, cmd := m.sendComposer()
			m = next.(model)
			require.NotNil(t, cmd)
			_, duplicate := m.sendComposer()
			require.Nil(t, duplicate)
			result := cmd()
			require.Equal(t, "/sessions/coder/messages", path)
			require.Equal(t, "Please check the edge cases.", body)
			if !fail {
				m.composer.SetValue("newer draft")
			}
			next, _ = m.Update(result)
			m = next.(model)
			if fail {
				require.Equal(t, body, m.composer.Value())
				require.NotNil(t, m.dialog)
				require.Contains(t, m.dialog.detail, "delivery unavailable")
			} else {
				require.Equal(t, "newer draft", m.composer.Value(), "a successful send must not clear subsequent typing")
				require.Contains(t, m.notice, "Queued to build")
			}
		})
	}
}

func TestUnavailableAgentNeverReceivesOrLosesDraft(t *testing.T) {
	m := messageExample()
	m.sessions = nil
	m.composer.SetValue("retain me")
	next, cmd := m.sendComposer()
	m = next.(model)
	require.Nil(t, cmd)
	require.Equal(t, "retain me", m.composer.Value())
	require.Contains(t, m.composerError, "no running session")
	m.composer.SetValue(strings.Repeat("é", 9000))
	next, cmd = m.sendComposer()
	require.Nil(t, cmd)
	require.Contains(t, next.(model).composerError, "16384 bytes")
}

func TestFocusedComposerFitsSmallAndExpandedLayouts(t *testing.T) {
	for _, size := range [][2]int{{72, 18}, {80, 24}, {120, 36}} {
		m := messageExample()
		m.width, m.height = size[0], size[1]
		m.focusComposer()
		m.composer.SetValue(strings.Repeat("agent feedback 長い ", 30))
		for _, full := range []bool{false, true} {
			m.fullscreen = full
			view := m.View()
			require.LessOrEqual(t, lipgloss.Width(view), m.width)
			require.LessOrEqual(t, lipgloss.Height(view), m.height)
		}
	}
}
