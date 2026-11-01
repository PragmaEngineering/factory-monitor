package dashboard

// Pages select the information on screen; panes select where navigation acts.
// Keep the selected execution and agent when moving between pages.
type monitorPage uint8

const (
	homePage monitorPage = iota
	messagesPage
	logsPage
)

func (m *model) showPage(page monitorPage) {
	m.saveComposer()
	m.composer.Blur()
	m.page = page
	m.fullscreen = false
	m.inboxAgent, m.inboxSelected = "", ""
	switch page {
	case homePage:
		m.focus = 1
	case messagesPage:
		m.focus, m.inboxAgent = 1, "@operator"
		m.loadComposer()
	case logsPage:
		m.focus = 2
		m.tab, m.scroll, m.follow = 0, 0, true
		m.ensureAgentSelection()
		m.rebuildLogs()
	}
}

func (m model) visiblePanes() []int {
	switch m.page {
	case messagesPage:
		if m.inboxAgent != "@operator" && m.inboxAgent != "" {
			return []int{1, 3, 4}
		}
		return []int{1, 3}
	case logsPage:
		return []int{2}
	default:
		return []int{0, 1, 3}
	}
}

func (m *model) cyclePane(direction int) {
	panes := m.visiblePanes()
	for i, pane := range panes {
		if pane == m.focus {
			m.focus = panes[(i+direction+len(panes))%len(panes)]
			return
		}
	}
	m.focus = panes[0]
}

func (m model) paneTitle(pane int) string {
	if m.page == logsPage {
		return m.detailTitle()
	}
	if m.page == messagesPage {
		switch pane {
		case 1:
			return "PARTICIPANTS"
		case 3:
			return "INBOX · " + m.recipientLabel()
		case 4:
			return "MESSAGE · " + m.recipientLabel()
		}
		return ""
	}
	switch pane {
	case 0:
		return "EXECUTIONS"
	case 1:
		return "WORKFLOW"
	case 3:
		return "OPERATOR INBOX"
	}
	return ""
}

func (m model) paneLines(pane, width, height int) []string {
	switch pane {
	case 0:
		return m.executionLines(width, height)
	case 1:
		if m.page == messagesPage {
			return m.recipientLines(width, height)
		}
		return m.workflowLines(width, height)
	case 2:
		return m.detailLines(width, height)
	case 3:
		return m.inboxLines(width, height)
	case 4:
		return m.composerLines(width, height)
	}
	return nil
}

func (m *model) ensureAgentSelection() {
	// A decision can be the current workflow step, but the communication
	// selection must point to an agent. Prefer the preceding agent.
	steps := m.steps()
	if len(steps) > 0 {
		selected := max(0, min(m.selectedStep, len(steps)-1))
		for selected > 0 && steps[selected].Run == nil {
			selected--
		}
		for selected < len(steps)-1 && steps[selected].Run == nil {
			selected++
		}
		m.selectedStep = selected
	}
}
