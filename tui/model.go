package tui

import (
	"fmt"
	"strings"

	"lazysm2/sm2"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	docStyle     = lipgloss.NewStyle().Margin(1, 2)
	titleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	runningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	stoppedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	panelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	popupStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).BorderForeground(lipgloss.Color("62")).Background(lipgloss.Color("236"))
	modeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	// logMarkStyle renders the per-entry gutter mark "│" in the log panel.
	// Wrapped continuation lines have no mark, making entry boundaries obvious.
	logMarkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

//- MESSAGES -----------------------------------------------------------------

type statusMessage struct{ message string }
type errorMessage struct{ err error }
type statusRefreshedMsg struct {
	status  sm2.Status
	warning string
}
type logStartMsg struct {
	serviceName string
	path        string
	output      string
	err         error
}
type logReadMsg struct {
	serviceName string
	session     int
	lines       []string
	offset      int64
	initialized bool
	err         error
}
type debugOutputMsg struct {
	serviceName string
	output      string
	err         error
}
type actionOutputMsg struct {
	session  int
	name     string
	verb     string
	pastVerb string
	output   string
	err      error
}
type actionStreamStartedMsg struct {
	session  int
	name     string
	verb     string
	pastVerb string
	lines    <-chan string
	done     <-chan actionOutputMsg
}
type actionStreamLineMsg struct {
	session int
	name    string
	verb    string
	line    string
	lines   <-chan string
}
type actionStreamClosedMsg struct {
	session int
}

func (e errorMessage) Error() string { return e.err.Error() }

type panel int

const (
	profilesPanel panel = iota
	runningPanel
	stoppedPanel
)

type popupKind int

const (
	noPopup popupKind = iota
	helpPopup
	actionOutputPopup
)

type popupState struct {
	kind    popupKind
	title   string
	content string
}

var panelShortcuts = map[string]panel{
	"1": profilesPanel,
	"2": runningPanel,
	"3": stoppedPanel,
}

// panelGeometry holds the computed terminal positions of the three list panels.
type panelGeometry struct {
	leftX      int    // first terminal column occupied by the left panels
	rightBound int    // first terminal column past the left panels (exclusive)
	top        [3]int // absolute top row of each panel (indexed by panel constant)
	height     [3]int // total height (borders included) of each panel
}

type model struct {
	sm2 sm2Client

	status          sm2.Status
	profiles        []string
	profileServices map[string][]string
	runningServices []sm2.Service
	stoppedServices []sm2.Service

	activePanel panel
	cursors     map[panel]int
	filters     map[panel]string
	filtering   bool

	vpActive        bool
	vpRestart       bool // true when R was pressed (restart), false when S (start)
	vpField         int  // 0 = version, 1 = port
	vpService       string
	vpVersionInput  textinput.Model
	vpPortInput     textinput.Model

	statusMessage  string
	popup          popupState
	width          int
	height         int
	logContent     string
	logFollowing   bool // true while the log panel auto-scrolls to new output
	selectedLog    string
	logPath        string
	logOffset      int64
	logInitialized bool
	logSession     int
	debugContent   string
	selectedDebug  string
	debugLoading   bool
	actionSession  int
	viewport       viewport.Model
}

func initialModel() model {
	vp := viewport.New(0, 0)
	return model{
		sm2:      realSM2Client{},
		cursors:  make(map[panel]int),
		filters:  make(map[panel]string),
		viewport: vp,
	}
}

func (m model) Init() tea.Cmd {
	return refreshStatusCommand(m.sm2)
}

func (m *model) updateServiceLists() {
	m.runningServices = []sm2.Service{}
	m.stoppedServices = []sm2.Service{}
	for _, s := range m.status.Services {
		if s.IsRunning {
			m.runningServices = append(m.runningServices, s)
		} else {
			m.stoppedServices = append(m.stoppedServices, s)
		}
	}
	m.profiles = m.status.Profiles
	m.profileServices = m.status.ProfileServices
	if m.profileServices == nil {
		m.profileServices = make(map[string][]string)
	}
}

func filterItems(items []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return items
	}

	filtered := []string{}
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterServices(services []sm2.Service, query string) []sm2.Service {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return services
	}

	filtered := []sm2.Service{}
	for _, service := range services {
		if strings.Contains(strings.ToLower(service.Name), query) {
			filtered = append(filtered, service)
		}
	}
	return filtered
}

func (m model) panelFilter(p panel) string {
	if m.filters == nil {
		return ""
	}
	return m.filters[p]
}

func (m *model) setPanelFilter(p panel, query string) {
	if m.filters == nil {
		m.filters = make(map[panel]string)
	}
	m.filters[p] = query
}

func (m model) filteredProfiles() []string {
	return filterItems(m.profiles, m.panelFilter(profilesPanel))
}

func (m model) filteredRunningServices() []sm2.Service {
	return filterServices(m.runningServices, m.panelFilter(runningPanel))
}

func (m model) filteredStoppedServices() []sm2.Service {
	return filterServices(m.selectedProfileStoppedServices(), m.panelFilter(stoppedPanel))
}

func (m model) selectedProfileStoppedServices() []sm2.Service {
	profileName := m.selectedProfileName()
	if profileName == "" {
		return nil
	}

	running := m.runningServiceSet()
	byName := make(map[string]sm2.Service, len(m.status.Services))
	for _, service := range m.status.Services {
		byName[service.Name] = service
	}

	stopped := []sm2.Service{}
	for _, serviceName := range m.profileServices[profileName] {
		if _, ok := running[serviceName]; ok {
			continue
		}
		service, ok := byName[serviceName]
		if !ok {
			service = sm2.Service{Name: serviceName, PID: "0", Status: "STOPPED"}
		}
		service.IsRunning = false
		stopped = append(stopped, service)
	}
	return stopped
}

func serviceNames(services []sm2.Service) []string {
	names := []string{}
	for _, service := range services {
		names = append(names, service.Name)
	}
	return names
}

func (m model) filteredRunningServiceNames() []string {
	return serviceNames(m.filteredRunningServices())
}

func (m model) filteredStoppedServiceNames() []string {
	return serviceNames(m.filteredStoppedServices())
}

func (m *model) clampCursor(p panel) {
	max := 0
	switch p {
	case profilesPanel:
		max = len(m.filteredProfiles()) - 1
	case runningPanel:
		max = len(m.filteredRunningServices()) - 1
	case stoppedPanel:
		max = len(m.filteredStoppedServices()) - 1
	}
	if m.cursors == nil {
		m.cursors = make(map[panel]int)
	}
	if m.cursors[p] > max {
		m.cursors[p] = max
	}
	if m.cursors[p] < 0 {
		m.cursors[p] = 0
	}
}

func (m *model) clampCursors() {
	m.clampCursor(profilesPanel)
	m.clampCursor(runningPanel)
	m.clampCursor(stoppedPanel)
}

func (m model) selectedProfileName() string {
	profiles := m.filteredProfiles()
	if len(profiles) == 0 || m.cursors[profilesPanel] >= len(profiles) {
		return ""
	}
	return profiles[m.cursors[profilesPanel]]
}

func (m model) selectedServiceName() string {
	switch m.activePanel {
	case runningPanel:
		services := m.filteredRunningServices()
		if len(services) > 0 {
			return services[m.cursors[runningPanel]].Name
		}
	case stoppedPanel:
		services := m.filteredStoppedServices()
		if len(services) > 0 && m.cursors[stoppedPanel] < len(services) {
			return services[m.cursors[stoppedPanel]].Name
		}
	}
	return ""
}

func (m model) selectedActionName() string {
	if m.activePanel == profilesPanel {
		return m.selectedProfileName()
	}
	return m.selectedServiceName()
}

func (m model) runningServiceSet() map[string]struct{} {
	running := make(map[string]struct{}, len(m.runningServices))
	for _, service := range m.runningServices {
		running[service.Name] = struct{}{}
	}
	return running
}

func (m *model) closePopup() {
	m.popup = popupState{}
}

func (m *model) showPopup(kind popupKind, title, content string) {
	m.popup = popupState{
		kind:    kind,
		title:   title,
		content: content,
	}
}

func (m *model) stopLogCommand() {
	m.logSession++
	m.selectedLog = ""
	m.logPath = ""
	m.logOffset = 0
	m.logInitialized = false
	m.logFollowing = false
}

func (m model) loadSelectedServiceDebug() (model, tea.Cmd) {
	m.clampCursor(m.activePanel)
	serviceName := m.selectedServiceName()
	if serviceName == "" {
		m.stopLogCommand()
		m.selectedDebug = ""
		if m.activePanel == profilesPanel {
			m.debugContent = ""
			m.viewport.SetContent(m.debugContent)
		} else {
			m.debugContent = "Select a running or stopped service to show debug output."
			m.viewport.SetContent(m.debugContent)
		}
		m.debugLoading = false
		return m, nil
	}
	if m.selectedDebug == serviceName && (m.debugLoading || m.debugContent != "") && m.selectedLog == "" {
		return m, nil
	}

	m.stopLogCommand()
	m.selectedDebug = serviceName
	m.debugContent = fmt.Sprintf("Loading debug output for %s...", serviceName)
	m.viewport.SetContent(m.debugContent)
	m.debugLoading = true
	return m, debugServiceCmd(m.sm2, serviceName)
}

func (m model) updateActiveFilter(query string) (model, tea.Cmd) {
	m.setPanelFilter(m.activePanel, query)
	if m.cursors == nil {
		m.cursors = make(map[panel]int)
	}
	m.cursors[m.activePanel] = 0
	return m.loadSelectedServiceDebug()
}

func (m model) updateFilterInput(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.stopLogCommand()
		return m, tea.Quit
	case "esc", "enter":
		m.filtering = false
		return m, nil
	case "backspace", "ctrl+h":
		query := []rune(m.panelFilter(m.activePanel))
		if len(query) > 0 {
			query = query[:len(query)-1]
			return m.updateActiveFilter(string(query))
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		return m.updateActiveFilter(m.panelFilter(m.activePanel) + string(msg.Runes))
	}
	return m, nil
}

func (m model) selectedService() *sm2.Service {
	switch m.activePanel {
	case runningPanel:
		services := m.filteredRunningServices()
		if len(services) > 0 {
			s := services[m.cursors[runningPanel]]
			return &s
		}
	case stoppedPanel:
		services := m.filteredStoppedServices()
		if len(services) > 0 && m.cursors[stoppedPanel] < len(services) {
			s := services[m.cursors[stoppedPanel]]
			return &s
		}
	}
	return nil
}

func newVPInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 64
	return ti
}

func (m *model) clearVPInput() {
	m.vpActive = false
	m.vpRestart = false
	m.vpField = 0
	m.vpService = ""
	m.vpVersionInput.Blur()
	m.vpPortInput.Blur()
}

func (m model) updateVPInput(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.stopLogCommand()
		return m, tea.Quit
	case "esc":
		m.clearVPInput()
		return m, nil
	case "tab":
		m.vpField = 1 - m.vpField
		if m.vpField == 0 {
			m.vpVersionInput.Focus()
			m.vpPortInput.Blur()
		} else {
			m.vpPortInput.Focus()
			m.vpVersionInput.Blur()
		}
		return m, nil
	case "enter":
		version, port, restart := m.vpVersionInput.Value(), m.vpPortInput.Value(), m.vpRestart
		name := m.vpService
		m.clearVPInput()
		if name != "" {
			target := name
			if version != "" {
				target = name + ":" + version
			}
			m.actionSession++
			m.stopLogCommand()
			m.selectedDebug = ""
			m.debugLoading = false
			if restart {
				m.statusMessage = fmt.Sprintf("Restarting '%s'...", target)
				m.showPopup(actionOutputPopup, fmt.Sprintf("Restarting: %s", target), m.statusMessage)
				return m, restartServiceWithVersionAndPortCmd(m.sm2, m.actionSession, name, version, port)
			}
			m.statusMessage = fmt.Sprintf("Starting '%s'...", target)
			m.showPopup(actionOutputPopup, fmt.Sprintf("Starting: %s", target), m.statusMessage)
			return m, startServiceWithVersionAndPortCmd(m.sm2, m.actionSession, name, version, port)
		}
		return m, nil
	}
	var cmd tea.Cmd
	if m.vpField == 0 {
		m.vpVersionInput, cmd = m.vpVersionInput.Update(msg)
	} else {
		m.vpPortInput, cmd = m.vpPortInput.Update(msg)
	}
	return m, cmd
}

// computePanelGeometry derives the panel positions from the current terminal
// size using the same arithmetic as baseView. Returns false when the terminal
// is too small to render a normal layout.
func (m model) computePanelGeometry() (panelGeometry, bool) {
	if m.width == 0 || m.height == 0 {
		return panelGeometry{}, false
	}
	hMargin, vMargin := docStyle.GetFrameSize()
	workingWidth := m.width - hMargin
	workingHeight := m.height - vMargin
	const headerHeight, footerHeight = 1, 1
	availableHeight := workingHeight - headerHeight - footerHeight - 2
	if availableHeight < 10 {
		return panelGeometry{}, false
	}
	leftWidth := workingWidth / 3
	const infoHeight = 7 // must match baseView
	remainingHeight := availableHeight - infoHeight
	panelHeight := remainingHeight / 3
	lastPanelHeight := remainingHeight - panelHeight*2

	leftX := hMargin / 2
	contentTop := vMargin/2 + headerHeight + 1 // top margin + header + blank separator
	return panelGeometry{
		leftX:      leftX,
		rightBound: leftX + leftWidth - 2,
		top: [3]int{
			contentTop + infoHeight,
			contentTop + infoHeight + panelHeight,
			contentTop + infoHeight + panelHeight*2,
		},
		height: [3]int{panelHeight, panelHeight, lastPanelHeight},
	}, true
}

// handlePanelClick activates panel p and moves the cursor to the item at
// rowInPanel (0 = top border of the panel).
func (m model) handlePanelClick(p panel, rowInPanel int, g panelGeometry) (tea.Model, tea.Cmd) {
	// Compute filter offset from pre-click state (what was actually rendered).
	filterOffset := 0
	if (m.filtering && m.activePanel == p) || m.panelFilter(p) != "" {
		filterOffset = 1
	}
	m.activePanel = p
	rowInContent := rowInPanel - 1 - filterOffset // skip top border + optional filter row
	if rowInContent < 0 {
		// Clicked on border or filter prompt — just activate the panel.
		return m.loadSelectedServiceDebug()
	}
	maxItems := panelContentHeight(g.height[p]) - filterOffset
	m.cursors[p] = scrollStart(m.cursors[p], maxItems) + rowInContent
	m.clampCursor(p)
	return m.loadSelectedServiceDebug()
}

// handlePanelScroll activates panel p and moves its cursor one step up or down.
func (m model) handlePanelScroll(p panel, up bool) (tea.Model, tea.Cmd) {
	m.activePanel = p
	if up {
		if m.cursors[p] > 0 {
			m.cursors[p]--
		}
	} else {
		var maxIdx int
		switch p {
		case profilesPanel:
			maxIdx = len(m.filteredProfiles()) - 1
		case runningPanel:
			maxIdx = len(m.filteredRunningServices()) - 1
		case stoppedPanel:
			maxIdx = len(m.filteredStoppedServices()) - 1
		}
		if m.cursors[p] < maxIdx {
			m.cursors[p]++
		}
	}
	return m.loadSelectedServiceDebug()
}

// footerHelp returns the normal footer hint text.
func (m model) footerHelp() string {
	return "'tab'/'1-3' panel, 'f' filter, 's/S/r/R/x/X' start/ver/restart/restart-ver/stop/stop-all, 'l' logs, 'G' follow, 'q' quit."
}

func (m model) syncViewport() model {
	if m.width == 0 || m.height == 0 {
		return m
	}
	hMargin, vMargin := docStyle.GetFrameSize()
	workingWidth := m.width - hMargin
	workingHeight := m.height - vMargin
	// Header and footer are each one truncated line; baseView adds two blank
	// separator rows between them and the main content area.
	const headerHeight, footerHeight = 1, 1
	availableHeight := workingHeight - headerHeight - footerHeight - 2
	rightWidth := workingWidth - (workingWidth / 3)

	m.viewport.Width = panelContentWidth(rightWidth - 2)
	m.viewport.Height = panelContentHeight(availableHeight)
	return m
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.syncViewport()
		return m, nil
	case tea.MouseMsg:
		g, ok := m.computePanelGeometry()
		onLeft := ok && msg.X >= g.leftX && msg.X < g.rightBound
		switch msg.Type {
		case tea.MouseLeft:
			if onLeft {
				for _, p := range []panel{profilesPanel, runningPanel, stoppedPanel} {
					if msg.Y >= g.top[p] && msg.Y < g.top[p]+g.height[p] {
						return m.handlePanelClick(p, msg.Y-g.top[p], g)
					}
				}
			}
		case tea.MouseWheelUp, tea.MouseWheelDown:
			if onLeft {
				up := msg.Type == tea.MouseWheelUp
				for _, p := range []panel{profilesPanel, runningPanel, stoppedPanel} {
					if msg.Y >= g.top[p] && msg.Y < g.top[p]+g.height[p] {
						return m.handlePanelScroll(p, up)
					}
				}
			}
			// Scrolling over the right (log/debug) panel.
			if m.selectedLog != "" && msg.Type == tea.MouseWheelUp {
				m.logFollowing = false
			}
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if m.selectedLog != "" && msg.Type == tea.MouseWheelDown && m.viewport.AtBottom() {
			m.logFollowing = true
		}
		return m, cmd
	case tea.KeyMsg:
		if msg.String() == "pgup" || msg.String() == "pgdown" {
			if msg.String() == "pgup" && m.selectedLog != "" {
				m.logFollowing = false
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			if msg.String() == "pgdown" && m.selectedLog != "" && m.viewport.AtBottom() {
				m.logFollowing = true
			}
			return m, cmd
		}
		if m.popup.kind != noPopup {
			switch m.popup.kind {
			case helpPopup:
				m.closePopup()
				return m, nil
			case actionOutputPopup:
				switch msg.String() {
				case "ctrl+c", "q":
					m.stopLogCommand()
					return m, tea.Quit
				case "esc", "enter":
					m.closePopup()
				}
				return m, nil
			}
		}
		if m.filtering {
			return m.updateFilterInput(msg)
		}
		if m.vpActive {
			return m.updateVPInput(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.stopLogCommand()
			return m, tea.Quit
		case "tab":
			m.activePanel = (m.activePanel + 1) % 3
			return m.loadSelectedServiceDebug()
		case "1", "2", "3":
			m.activePanel = panelShortcuts[msg.String()]
			return m.loadSelectedServiceDebug()
		case "f":
			m.filtering = true
			return m, nil
		case "up", "k":
			if m.cursors[m.activePanel] > 0 {
				m.cursors[m.activePanel]--
				return m.loadSelectedServiceDebug()
			}
		case "down", "j":
			max := 0
			switch m.activePanel {
			case profilesPanel:
				max = len(m.filteredProfiles()) - 1
			case runningPanel:
				max = len(m.filteredRunningServices()) - 1
			case stoppedPanel:
				max = len(m.filteredStoppedServices()) - 1
			}
			if m.cursors[m.activePanel] < max {
				m.cursors[m.activePanel]++
				return m.loadSelectedServiceDebug()
			}
		case "s":
			name := m.selectedActionName()
			if name != "" {
				m.actionSession++
				m.statusMessage = fmt.Sprintf("Starting '%s'...", name)
				m.stopLogCommand()
				m.selectedDebug = ""
				m.debugLoading = false
				m.showPopup(actionOutputPopup, fmt.Sprintf("Starting: %s", name), fmt.Sprintf("Starting '%s'...", name))
				if m.activePanel == profilesPanel {
					services := m.profileServices[name]
					return m, startProfileCmd(m.sm2, m.actionSession, name, services)
				}
				return m, startServiceCmd(m.sm2, m.actionSession, name)
			}
		case "r":
			name := m.selectedServiceName()
			if name != "" {
				m.actionSession++
				m.statusMessage = fmt.Sprintf("Restarting '%s'...", name)
				m.stopLogCommand()
				m.selectedDebug = ""
				m.debugLoading = false
				m.showPopup(actionOutputPopup, fmt.Sprintf("Restarting: %s", name), fmt.Sprintf("Restarting '%s'...", name))
				return m, restartServiceCmd(m.sm2, m.actionSession, name)
			}
		case "S":
			svc := m.selectedService()
			if svc != nil {
				m.vpActive = true
				m.vpRestart = false
				m.vpField = 0
				m.vpService = svc.Name
				m.vpVersionInput = newVPInput("version")
				m.vpVersionInput.SetValue(svc.Version)
				m.vpVersionInput.Focus()
				m.vpPortInput = newVPInput("port")
				m.vpPortInput.SetValue(svc.Port)
				return m, nil
			}
		case "R":
			svc := m.selectedService()
			if svc != nil {
				m.vpActive = true
				m.vpRestart = true
				m.vpField = 0
				m.vpService = svc.Name
				m.vpVersionInput = newVPInput("version")
				m.vpVersionInput.SetValue(svc.Version)
				m.vpVersionInput.Focus()
				m.vpPortInput = newVPInput("port")
				m.vpPortInput.SetValue(svc.Port)
				return m, nil
			}
		case "x":
			name := m.selectedActionName()
			if name != "" {
				m.statusMessage = fmt.Sprintf("Stopping '%s'...", name)
				m.stopLogCommand()
				m.selectedDebug = ""
				m.debugLoading = false
				m.showPopup(actionOutputPopup, fmt.Sprintf("Stopping: %s", name), fmt.Sprintf("Stopping '%s'...", name))
				return m, stopServiceCmd(m.sm2, name)
			}
		case "X":
			m.statusMessage = "Stopping all services..."
			m.stopLogCommand()
			m.selectedDebug = ""
			m.debugLoading = false
			m.showPopup(actionOutputPopup, "Stopping: all services", "Stopping all services...")
			return m, stopAllCmd(m.sm2)
		case "l":
			name := m.selectedServiceName()
			if name != "" {
				m.stopLogCommand()
				m.logContent = ""
				m.viewport.SetContent(m.logContent)
				m.selectedLog = name
				m.logFollowing = true
				m.logSession++
				m.statusMessage = fmt.Sprintf("Finding log file for '%s'...", name)
				return m, startLogCmd(m.sm2, name)
			}
		case "G":
			m.viewport.GotoBottom()
			if m.selectedLog != "" {
				m.logFollowing = true
			}
			return m, nil
		case "?":
			m.showPopup(helpPopup, "Help", helpContent())
			return m, nil
		}

	case statusRefreshedMsg:
		m.status = msg.status
		m.updateServiceLists()
		if msg.warning != "" {
			m.statusMessage = errorStyle.Render(fmt.Sprintf("Service status refreshed. %s", msg.warning))
		} else {
			m.statusMessage = "Service status refreshed."
		}
		m.clampCursors()
		return m.loadSelectedServiceDebug()

	case statusMessage:
		m.statusMessage = msg.message
		return m, refreshStatusCommand(m.sm2)

	case actionStreamStartedMsg:
		if msg.session != m.actionSession {
			return m, nil
		}
		return m, tea.Batch(waitForActionLine(msg.session, msg.name, msg.verb, msg.lines), waitForActionDone(msg.session, msg.done))

	case actionStreamLineMsg:
		if msg.session != m.actionSession {
			return m, nil
		}
		if m.popup.kind == actionOutputPopup {
			if strings.TrimSpace(m.popup.content) == "" {
				m.popup.content = msg.line
			} else {
				m.popup.content += "\n" + msg.line
			}
		}
		m.statusMessage = fmt.Sprintf("%s '%s': %s", msg.verb, msg.name, msg.line)
		return m, waitForActionLine(msg.session, msg.name, msg.verb, msg.lines)

	case actionStreamClosedMsg:
		if msg.session != m.actionSession {
			return m, nil
		}
		return m, nil

	case actionOutputMsg:
		if msg.session != 0 && msg.session != m.actionSession {
			return m, nil
		}
		// On error, show the action output popup so the user can inspect logs.
		if msg.err != nil {
			m.showPopup(actionOutputPopup, fmt.Sprintf("%s: %s", msg.verb, msg.name), actionOutputContent(msg))
			m.statusMessage = errorStyle.Render(fmt.Sprintf("Error %s '%s'.", strings.ToLower(msg.verb), msg.name))
			return m, refreshStatusCommand(m.sm2)
		}
		m.closePopup()
		m.statusMessage = fmt.Sprintf("'%s' %s.", msg.name, msg.pastVerb)
		return m, refreshStatusCommand(m.sm2)

	case errorMessage:
		m.statusMessage = errorStyle.Render(fmt.Sprintf("Error: %s", msg.Error()))
		return m, nil

	case logStartMsg:
		if msg.serviceName != m.selectedLog {
			return m, nil
		}
		if msg.err != nil {
			m.logContent = fmt.Sprintf("Unable to find stdout.log from debug output: %s", msg.err)
			if cleaned := cleanDebugOutput(msg.output); cleaned != "" {
				m.logContent += "\n\n" + cleaned
			}
			m.viewport.SetContent(m.logContent)
			return m, nil
		}
		m.logPath = msg.path
		m.logOffset = 0
		m.logInitialized = false
		m.logContent = fmt.Sprintf("Reading %s...", msg.path)
		m.viewport.SetContent(m.logContent)
		m.statusMessage = fmt.Sprintf("Showing logs for '%s'.", msg.serviceName)
		return m, waitForLogFile(msg.serviceName, m.logSession, m.logPath, m.logOffset, m.logInitialized)

	case logReadMsg:
		if msg.serviceName != m.selectedLog || msg.session != m.logSession {
			return m, nil
		}
		m.logOffset = msg.offset
		m.logInitialized = msg.initialized
		if msg.err != nil && (m.logContent == "" || strings.HasPrefix(m.logContent, "Reading ") || strings.HasPrefix(m.logContent, "Waiting for ")) {
			m.logContent = fmt.Sprintf("Waiting for %s: %s", m.logPath, msg.err)
			m.viewport.SetContent(m.logContent)
		}
		if len(msg.lines) > 0 {
			if strings.HasPrefix(m.logContent, "Reading ") || strings.HasPrefix(m.logContent, "Waiting for ") {
				m.logContent = ""
			}
			mark := logMarkStyle.Render("│") + " "
			for _, line := range msg.lines {
				m.logContent += mark + line + "\n"
			}
			m.logContent = capLines(m.logContent, maxLogLines)
			m.viewport.SetContent(m.logContent)
			if m.logFollowing {
				m.viewport.GotoBottom()
			}
		}
		return m, waitForLogFile(msg.serviceName, m.logSession, m.logPath, m.logOffset, m.logInitialized)

	case debugOutputMsg:
		if msg.serviceName != m.selectedDebug || m.selectedLog != "" {
			return m, nil
		}
		m.debugLoading = false
		m.debugContent = cleanDebugOutput(msg.output)
		if msg.err != nil {
			errorText := fmt.Sprintf("Error: %s", msg.err)
			if m.debugContent != "" {
				m.debugContent += "\n" + errorText
			} else {
				m.debugContent = errorText
			}
		}
		if m.debugContent == "" {
			m.debugContent = "No debug output."
		}
		m.viewport.SetContent(m.debugContent)
		return m, nil
	}
	return m, nil
}
