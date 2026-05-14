package the_stage

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"proxymaxxing/the_bouncer"
	"proxymaxxing/the_oracle"
)

var (
	tabStyle         = lipgloss.NewStyle().Padding(0, 2).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("238"))
	activeTabStyle   = tabStyle.BorderForeground(lipgloss.Color("69")).Foreground(lipgloss.Color("69")).Bold(true)
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	subTitleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	unselectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	statusOkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	panelBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238"))
)

type LogMsg the_bouncer.LogEvent

type Model struct {
	cfg        *the_oracle.Config
	configPath string
	activeTab  int

	serviceCursor int
	editingDest   bool
	destInput     textinput.Model

	inspServiceCursor int
	logs              []the_bouncer.LogEvent
	logChan           chan the_bouncer.LogEvent
	viewport          viewport.Model

	totalReq     int
	reroutedReq  int
	forwardedReq int

	width  int
	height int
}

func InitialModel(cfg *the_oracle.Config, configPath string, logChan chan the_bouncer.LogEvent) Model {
	vp := viewport.New(0, 0)
	vp.YPosition = 4

	ti := textinput.New()
	ti.Placeholder = "http://localhost:8081"
	ti.CharLimit = 156
	ti.Width = 40

	return Model{
		cfg:        cfg,
		configPath: configPath,
		activeTab:  0,
		destInput:  ti,
		logChan:    logChan,
		viewport:   vp,
	}
}

func waitForActivity(sub chan the_bouncer.LogEvent) tea.Cmd {
	return func() tea.Msg {
		return LogMsg(<-sub)
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		waitForActivity(m.logChan),
	)
}

func (m Model) saveConfig() {
	outData, err := yaml.Marshal(m.cfg)
	if err == nil {
		os.WriteFile(m.configPath, outData, 0644)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	if m.editingDest {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				m.editingDest = false
				m.cfg.Services[m.serviceCursor].RerouteDestination = m.destInput.Value()
				m.saveConfig()
				return m, nil
			case "esc":
				m.editingDest = false
				return m, nil
			}
		}
		m.destInput, cmd = m.destInput.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width/2 - 4
		m.viewport.Height = msg.Height - 6

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab", "right":
			m.activeTab = (m.activeTab + 1) % 2
			m.updateViewport()
		case "shift+tab", "left":
			m.activeTab = (m.activeTab - 1 + 2) % 2
			m.updateViewport()

		case "up", "k":
			if m.activeTab == 0 && m.serviceCursor > 0 {
				m.serviceCursor--
			} else if m.activeTab == 1 && m.inspServiceCursor > 0 {
				m.inspServiceCursor--
				m.updateViewport()
			}
		case "down", "j":
			if m.activeTab == 0 && m.serviceCursor < len(m.cfg.Services)-1 {
				m.serviceCursor++
			} else if m.activeTab == 1 && m.inspServiceCursor < len(m.cfg.Services)-1 {
				m.inspServiceCursor++
				m.updateViewport()
			}
		case " ":
			if m.activeTab == 0 && len(m.cfg.Services) > 0 {
				m.cfg.Services[m.serviceCursor].RerouteFlag = !m.cfg.Services[m.serviceCursor].RerouteFlag
				m.saveConfig()
			}
		case "i":
			if m.activeTab == 0 && len(m.cfg.Services) > 0 {
				m.editingDest = true
				m.destInput.SetValue(m.cfg.Services[m.serviceCursor].RerouteDestination)
				m.destInput.Focus()
				return m, textinput.Blink
			}
		}

	case LogMsg:
		m.logs = append(m.logs, the_bouncer.LogEvent(msg))
		m.totalReq++
		if msg.Local {
			m.reroutedReq++
		} else {
			m.forwardedReq++
		}
		m.updateViewport()
		return m, waitForActivity(m.logChan)
	}

	if m.activeTab == 1 {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) updateViewport() {
	if m.activeTab != 1 || len(m.cfg.Services) == 0 {
		return
	}

	selectedSvc := m.cfg.Services[m.inspServiceCursor].Name
	var b strings.Builder
	count := 0

	for _, l := range m.logs {
		if l.Service == selectedSvc {
			count++
			prefix := "[R]"
			if l.Local {
				prefix = "[L]"
			}

			statusStr := fmt.Sprintf("%d", l.Status)
			if l.Status >= 200 && l.Status < 400 {
				statusStr = statusOkStyle.Render(statusStr)
			} else {
				statusStr = statusErrStyle.Render(statusStr)
			}

			timeStr := subTitleStyle.Render(l.Time.Format("15:04:05"))
			b.WriteString(fmt.Sprintf("%s %s %s %s -> %s\n", timeStr, prefix, statusStr, l.Path, l.Dest))
		}
	}

	if count == 0 {
		b.WriteString(subTitleStyle.Render("No requests captured yet..."))
	}

	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	if m.width == 0 {
		return "Setting the stage..."
	}

	tabs := []string{"The Services", "The Inspector"}
	var renderedTabs []string
	for i, t := range tabs {
		if m.activeTab == i {
			renderedTabs = append(renderedTabs, activeTabStyle.Render(t))
		} else {
			renderedTabs = append(renderedTabs, tabStyle.Render(t))
		}
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	var content string

	if m.activeTab == 0 {
		var b strings.Builder
		addr := fmt.Sprintf("0.0.0.0:%d", m.cfg.Port)
		b.WriteString(titleStyle.Render("Service Manager") + " " + subTitleStyle.Render(fmt.Sprintf("(Serving at %s)", addr)) + "\n")
		b.WriteString(subTitleStyle.Render(fmt.Sprintf("Total Requests: %d | Rerouted: %d | Forwarded: %d", m.totalReq, m.reroutedReq, m.forwardedReq)) + "\n")
		b.WriteString(subTitleStyle.Render("(Press Space to toggle hijack, 'i' to edit destination, Up/Down to navigate)") + "\n\n")

		for i, svc := range m.cfg.Services {
			cursor := "  "
			style := unselectedStyle
			if m.serviceCursor == i {
				cursor = "> "
				style = selectedStyle
			}

			status := "[ ] Remote  "
			if svc.RerouteFlag {
				status = "[x] Hijacked"
			}

			b.WriteString(style.Render(fmt.Sprintf("%s%s %-30s", cursor, status, svc.Name)))
			b.WriteString(subTitleStyle.Render(fmt.Sprintf(" (%d routes)\n", len(svc.HijackedRoutes))))
			if m.serviceCursor == i {
				b.WriteString(subTitleStyle.Render(fmt.Sprintf("      Prefix: %s\n", svc.BasePath)))
				b.WriteString(subTitleStyle.Render(fmt.Sprintf("      Origin: %s\n", svc.RouteOrigin)))
				if m.editingDest {
					b.WriteString(subTitleStyle.Render("      Dest:   ") + m.destInput.View() + "\n")
				} else {
					b.WriteString(subTitleStyle.Render(fmt.Sprintf("      Dest:   %s\n", svc.RerouteDestination)))
				}
			}
		}
		content = b.String()

	} else {
		var leftB strings.Builder
		leftB.WriteString(titleStyle.Render("Services") + "\n\n")

		counts := make(map[string]int)
		for _, l := range m.logs {
			counts[l.Service]++
		}

		for i, svc := range m.cfg.Services {
			cursor := "  "
			style := unselectedStyle
			if m.inspServiceCursor == i {
				cursor = "> "
				style = selectedStyle
			}
			leftB.WriteString(style.Render(fmt.Sprintf("%s%-25s %d reqs\n", cursor, svc.Name, counts[svc.Name])))
		}

		leftPane := panelBorderStyle.Width(m.width/3 - 2).Height(m.height - 5).Render(leftB.String())

		rightPaneTitle := titleStyle.Render(fmt.Sprintf("Logs: %s", m.cfg.Services[m.inspServiceCursor].Name))
		rightPaneContent := lipgloss.JoinVertical(lipgloss.Left, rightPaneTitle, m.viewport.View())
		rightPane := panelBorderStyle.Width(m.width - (m.width / 3) - 2).Height(m.height - 5).Render(rightPaneContent)

		content = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, "", content)
}
