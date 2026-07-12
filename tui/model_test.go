package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"lazysm2/sm2"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testModel() model {
	status := sm2.Status{
		Services: []sm2.Service{
			{Name: "SERVICE-A", Version: "1.0.0", PID: "10001", Port: "8001", Status: "PASS", IsRunning: true},
			{Name: "SERVICE-B", Version: "2.0.0", PID: "0", Port: "8002", Status: "FAIL", IsRunning: false},
			{Name: "SERVICE-C", Version: "3.0.0", PID: "10003", Port: "8003", Status: "PASS", IsRunning: true},
		},
		Profiles: []string{"PROFILE-X", "PROFILE-Y"},
		ProfileServices: map[string][]string{
			"PROFILE-X": {"SERVICE-A", "SERVICE-B"},
			"PROFILE-Y": {"SERVICE-C"},
		},
	}
	m := initialModel()
	m.status = status
	m.updateServiceLists()
	return m
}

type fakeSM2Client struct {
	status sm2.Status
	err    error
}

func (f fakeSM2Client) GetStatus() (sm2.Status, error) {
	return f.status, f.err
}

func (f fakeSM2Client) StartServiceOutput(string) (string, error) {
	return "", nil
}

func (f fakeSM2Client) StartServiceOutputStream(_ string, _ string, onLine func(string)) (string, error) {
	if onLine != nil {
		onLine("starting line")
	}
	return "starting line\n", nil
}

func (f fakeSM2Client) StopServiceOutput(string) (string, error) {
	return "", nil
}

func (f fakeSM2Client) StopAllOutput() (string, error) {
	return "", nil
}

func (f fakeSM2Client) DebugService(string) (string, error) {
	return "", nil
}

func (f fakeSM2Client) StartMode() string {
	return "ONLINE"
}

func TestPanelViewFitsRequestedDimensions(t *testing.T) {
	m := testModel()

	view := m.panelView("Running Services", []string{"SERVICE-A", "SERVICE-B", "SERVICE-C"}, runningPanel, 24, 8)

	width, height := lipgloss.Size(view)
	if width != 24 {
		t.Fatalf("panel width = %d, want 24", width)
	}
	if height != 8 {
		t.Fatalf("panel height = %d, want 8", height)
	}
}

func TestStatusRefreshShowsProfileLoadWarning(t *testing.T) {
	m := initialModel()
	m.sm2 = fakeSM2Client{
		status: sm2.Status{
			Services:       []sm2.Service{{Name: "SERVICE-A", Status: "PASS", IsRunning: true}},
			ProfileLoadErr: errors.New("load profiles: unavailable"),
		},
	}

	msg := refreshStatusCommand(m.sm2)()
	updated, _ := m.Update(msg)
	got := updated.(model)

	if len(got.runningServices) != 1 || got.runningServices[0].Name != "SERVICE-A" {
		t.Fatalf("running services = %+v, want AUTH from partial status", got.runningServices)
	}
	if !strings.Contains(got.statusMessage, "load profiles: unavailable") {
		t.Fatalf("status message missing profile warning: %q", got.statusMessage)
	}
}

func TestPanelViewShowsShortcutNumber(t *testing.T) {
	m := testModel()

	view := m.panelView("Running Services", []string{"SERVICE-A"}, runningPanel, 24, 8)

	if !strings.Contains(view, "[2] Running Services") {
		t.Fatalf("panel title missing shortcut number:\n%s", view)
	}
}

func TestNumberKeysJumpToPanels(t *testing.T) {
	tests := []struct {
		key  string
		want panel
	}{
		{key: "1", want: profilesPanel},
		{key: "2", want: runningPanel},
		{key: "3", want: stoppedPanel},
	}

	for _, tt := range tests {
		m := testModel()

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
		got := updated.(model).activePanel

		if got != tt.want {
			t.Fatalf("key %q active panel = %d, want %d", tt.key, got, tt.want)
		}
	}
}

func TestFilterInputFiltersActivePanelCaseInsensitive(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	updated, cmd := updated.(model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("service-c")})
	got := updated.(model)
	services := got.filteredRunningServices()

	if !got.filtering {
		t.Fatal("expected model to stay in filter input mode")
	}
	if got.panelFilter(runningPanel) != "service-c" {
		t.Fatalf("filter = %q, want service-c", got.panelFilter(runningPanel))
	}
	if len(services) != 1 || services[0].Name != "SERVICE-C" {
		t.Fatalf("filtered running services = %+v, want SERVICE-C", services)
	}
	if got.selectedDebug != "SERVICE-C" {
		t.Fatalf("selected debug service = %q, want SERVICE-C", got.selectedDebug)
	}
	if cmd == nil {
		t.Fatal("expected debug command for filtered selected service")
	}
}

func TestFilteredPanelShowsFilterAndNoMatches(t *testing.T) {
	m := testModel()
	m.filters = map[panel]string{runningPanel: "missing"}
	m.filtering = true
	m.activePanel = runningPanel

	view := m.panelView("Running Services", m.filteredRunningServiceNames(), runningPanel, 32, 8)

	if !strings.Contains(view, "Filter: missing_") {
		t.Fatalf("panel missing active filter prompt:\n%s", view)
	}
	if !strings.Contains(view, "No matches") {
		t.Fatalf("panel missing no matches text:\n%s", view)
	}
}

func TestEscapeKeepsActiveFilter(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel
	m.filtering = true
	m.setPanelFilter(runningPanel, "service-c")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(model)

	if got.filtering {
		t.Fatal("expected escape to exit filter mode")
	}
	if got.panelFilter(runningPanel) != "service-c" {
		t.Fatalf("filter = %q, want service-c", got.panelFilter(runningPanel))
	}
}

func TestEnterKeepsActiveFilter(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel
	m.filtering = true
	m.setPanelFilter(runningPanel, "service-c")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	if got.filtering {
		t.Fatal("expected enter to exit filter mode")
	}
	if got.panelFilter(runningPanel) != "service-c" {
		t.Fatalf("filter = %q, want service-c", got.panelFilter(runningPanel))
	}
}

func TestStartUsesFilteredSelection(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel
	m.setPanelFilter(runningPanel, "service-c")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("expected start command for filtered selection")
	}
	if got.statusMessage != "Starting 'SERVICE-C'..." {
		t.Fatalf("status message = %q, want SERVICE-C start", got.statusMessage)
	}
	if got.popup.kind != actionOutputPopup {
		t.Fatalf("popup kind = %d, want action output", got.popup.kind)
	}
	if !strings.Contains(got.popup.content, "Starting 'SERVICE-C'...") {
		t.Fatalf("popup missing start progress:\n%s", got.popup.content)
	}
}

func TestActionOutputClosesPopupOnSuccess(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m.showPopup(actionOutputPopup, "Starting: PROFILE-X", "Starting PROFILE-X")

	updated, cmd := m.Update(actionOutputMsg{
		name:     "PROFILE-X",
		verb:     "Starting",
		pastVerb: "started",
		output:   "Starting SERVICE-A\nStarting SERVICE-B",
	})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("expected status refresh command after action output")
	}
	if got.statusMessage != "'PROFILE-X' started." {
		t.Fatalf("status message = %q, want profile started", got.statusMessage)
	}
	if got.popup.kind != noPopup {
		t.Fatalf("popup kind = %d, want no popup", got.popup.kind)
	}
}

func TestActionStreamLineAppendsToPopup(t *testing.T) {
	m := testModel()
	m.actionSession = 1
	m.showPopup(actionOutputPopup, "Starting: SERVICE-A", "Starting 'SERVICE-A'...")

	updated, cmd := m.Update(actionStreamLineMsg{
		session: 1,
		name:    "SERVICE-A",
		verb:    "Starting",
		line:    "Downloading dependencies",
		lines:   make(chan string),
	})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("expected command waiting for next streamed line")
	}
	if !strings.Contains(got.popup.content, "Starting 'SERVICE-A'...") {
		t.Fatalf("popup lost initial action text:\n%s", got.popup.content)
	}
	if !strings.Contains(got.popup.content, "Downloading dependencies") {
		t.Fatalf("popup missing streamed line:\n%s", got.popup.content)
	}
	if got.statusMessage != "Starting 'SERVICE-A': Downloading dependencies" {
		t.Fatalf("status message = %q", got.statusMessage)
	}
}

func TestCleanCommandOutputLineRemovesTerminalControls(t *testing.T) {
	got := cleanCommandOutputLine("\x1b[31mDownloading 10%\rDownloading 20%\x1b[0m\b")

	if got != "Downloading 20%" {
		t.Fatalf("cleaned line = %q, want latest progress text", got)
	}
}

func TestActionOutputShowsErrorsInPopup(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30

	updated, _ := m.Update(actionOutputMsg{
		name:     "SERVICE-A",
		verb:     "Stopping",
		pastVerb: "stopped",
		output:   "Stopping SERVICE-A failed",
		err:      os.ErrPermission,
	})
	view := updated.(model).popupView()

	if !strings.Contains(view, "Stopping SERVICE-A failed") {
		t.Fatalf("popup missing command output:\n%s", view)
	}
	if !strings.Contains(view, "Error: permission denied") {
		t.Fatalf("popup missing command error:\n%s", view)
	}
}

func TestSelectingServiceStartsDebugLoad(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("expected debug command when selected service changes")
	}
	if got.selectedDebug != "SERVICE-C" {
		t.Fatalf("selected debug service = %q, want SERVICE-C", got.selectedDebug)
	}
	if !strings.Contains(got.debugContent, "Loading debug output for SERVICE-C") {
		t.Fatalf("debug loading text = %q", got.debugContent)
	}
}

func TestDebugOutputShowsInRightPanel(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel
	m.selectedDebug = "SERVICE-A"

	updated, _ := m.Update(debugOutputMsg{serviceName: "SERVICE-A", output: "debug line 1\ndebug line 2"})
	view := updated.(model).logPanelView(40, 8)

	if !strings.Contains(view, "Debug: SERVICE-A") {
		t.Fatalf("debug panel title missing service name:\n%s", view)
	}
	if !strings.Contains(view, "debug line 1") || !strings.Contains(view, "debug line 2") {
		t.Fatalf("debug panel missing output:\n%s", view)
	}
}

func TestProfileSelectionShowsServiceStatusesInRightPanel(t *testing.T) {
	m := testModel()
	m.activePanel = profilesPanel

	view := m.logPanelView(48, 10)

	if !strings.Contains(view, "Profile: PROFILE-X") {
		t.Fatalf("profile panel title missing selected profile:\n%s", view)
	}
	if !strings.Contains(view, "running SERVICE-A") {
		t.Fatalf("profile panel missing running service:\n%s", view)
	}
	if !strings.Contains(view, "stopped SERVICE-B") {
		t.Fatalf("profile panel missing stopped service:\n%s", view)
	}
}

func TestProfileSelectionStatusUpdatesWhenRunningServicesChange(t *testing.T) {
	m := testModel()
	m.activePanel = profilesPanel
	m.status.Services = []sm2.Service{
		{Name: "SERVICE-A", Status: "FAIL", IsRunning: false},
		{Name: "SERVICE-B", Status: "PASS", IsRunning: true},
	}
	m.updateServiceLists()

	view := m.logPanelView(48, 10)

	if !strings.Contains(view, "stopped SERVICE-A") {
		t.Fatalf("profile panel did not compare SERVICE-A with running services:\n%s", view)
	}
	if !strings.Contains(view, "running SERVICE-B") {
		t.Fatalf("profile panel did not compare SERVICE-B with running services:\n%s", view)
	}
}

func TestStoppedPanelShowsSelectedProfileServicesThatAreNotRunning(t *testing.T) {
	m := testModel()
	m.activePanel = stoppedPanel

	got := m.filteredStoppedServiceNames()
	want := []string{"SERVICE-B"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stopped services = %+v, want %+v", got, want)
	}
}

func TestStoppedPanelChangesWithSelectedProfile(t *testing.T) {
	m := testModel()
	m.activePanel = stoppedPanel
	m.cursors[profilesPanel] = 1

	got := m.filteredStoppedServiceNames()

	if len(got) != 0 {
		t.Fatalf("stopped services = %+v, want none for PROFILE-Y", got)
	}
}

func TestDebugOutputHidesInstallCheckLine(t *testing.T) {
	m := testModel()
	m.selectedDebug = "SERVICE-A"

	updated, _ := m.Update(debugOutputMsg{serviceName: "SERVICE-A", output: "Checking .install file...\ndebug line"})
	got := updated.(model).debugContent

	if strings.Contains(got, "Checking .install file...") {
		t.Fatalf("debug output includes install check line: %q", got)
	}
	if got != "debug line" {
		t.Fatalf("debug output = %q, want debug line", got)
	}
}

func TestDebugOutputOnlyHidesInstallCheckWhenFirstLine(t *testing.T) {
	got := cleanDebugOutput("debug line\nChecking .install file...")

	if got != "debug line\nChecking .install file..." {
		t.Fatalf("debug output = %q", got)
	}
}

func TestReadLogFileReturnsInitialTailThenAppendedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout.log")
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0644); err != nil {
		t.Fatalf("write initial log: %v", err)
	}

	lines, offset, initialized, err := readLogFile(path, 0, false)
	if err != nil {
		t.Fatalf("read initial log: %v", err)
	}
	if !initialized {
		t.Fatal("expected log reader to be initialized")
	}
	if strings.Join(lines, "\n") != "first\nsecond" {
		t.Fatalf("initial lines = %#v", lines)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open log for append: %v", err)
	}
	if _, err := file.WriteString("third\n"); err != nil {
		t.Fatalf("append log: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}

	lines, _, _, err = readLogFile(path, offset, initialized)
	if err != nil {
		t.Fatalf("read appended log: %v", err)
	}
	if strings.Join(lines, "\n") != "third" {
		t.Fatalf("appended lines = %#v", lines)
	}
}

func TestViewFitsTerminalDimensions(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 80, height: 24},
		{width: 100, height: 30},
	} {
		for _, activePanel := range []panel{profilesPanel, runningPanel, stoppedPanel} {
			m := testModel()
			m.width = size.width
			m.height = size.height
			m.activePanel = activePanel

			view := m.View()

			width, height := lipgloss.Size(view)
			if width > m.width {
				t.Fatalf("view width = %d, want <= %d", width, m.width)
			}
			if height > m.height {
				t.Fatalf("view height = %d, want <= %d", height, m.height)
			}
		}
	}
}

func TestViewFitsTerminalDimensionsWithLongLogLine(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.selectedLog = "SERVICE-A"
	m.logContent = strings.Repeat("x", 500)

	view := m.View()

	width, height := lipgloss.Size(view)
	if width > m.width {
		t.Fatalf("view width = %d, want <= %d", width, m.width)
	}
	if height > m.height {
		t.Fatalf("view height = %d, want <= %d", height, m.height)
	}
	if !strings.Contains(view, "[2] Running Service") {
		t.Fatalf("view missing side panels:\n%s", view)
	}
}

func TestHelpRendersOverBaseView(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m.showPopup(helpPopup, "Help", helpContent())

	view := m.View()

	if !strings.Contains(view, "Press any key to close help.") {
		t.Fatalf("view missing help content:\n%s", view)
	}
	if !strings.Contains(view, "Service Manager") {
		t.Fatalf("view missing underlying header:\n%s", view)
	}
	if !strings.Contains(view, "[1] Profiles") {
		t.Fatalf("view missing underlying panels:\n%s", view)
	}

	width, height := lipgloss.Size(view)
	if width > m.width {
		t.Fatalf("view width = %d, want <= %d", width, m.width)
	}
	if height > m.height {
		t.Fatalf("view height = %d, want <= %d", height, m.height)
	}
}

func TestRestartKeyOpensPopupAndLaunchesCmd(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("expected restart command")
	}
	if got.popup.kind != actionOutputPopup {
		t.Fatalf("popup kind = %d, want actionOutputPopup", got.popup.kind)
	}
	if !strings.Contains(got.popup.title, "Restarting") {
		t.Fatalf("popup title = %q, want Restarting", got.popup.title)
	}
}

func TestRestartKeyIgnoredOnProfilesPanel(t *testing.T) {
	m := testModel()
	m.activePanel = profilesPanel

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when r pressed on profiles panel")
	}
	if got.popup.kind != noPopup {
		t.Fatalf("expected no popup on profiles panel, got kind %d", got.popup.kind)
	}
}

func TestVersionKeyEntersInputMode(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	got := updated.(model)

	if !got.vpActive {
		t.Fatal("expected vpActive to be true after S")
	}
	if got.vpService != "SERVICE-A" {
		t.Fatalf("vpService = %q, want SERVICE-A", got.vpService)
	}
	if got.vpVersionInput.Value() != "1.0.0" {
		t.Fatalf("vpVersionInput = %q, want 1.0.0 (pre-filled from service)", got.vpVersionInput.Value())
	}
	if got.vpPortInput.Value() != "8001" {
		t.Fatalf("vpPortInput = %q, want 8001 (pre-filled from service)", got.vpPortInput.Value())
	}
}

func TestVersionInputTypingUpdatesVersionField(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel
	m.vpActive = true
	m.vpService = "SERVICE-A"
	m.vpVersionInput = newVPInput("version")
	m.vpVersionInput.Focus()
	m.vpPortInput = newVPInput("port")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	updated, _ = updated.(model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	updated, _ = updated.(model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	got := updated.(model)

	if got.vpVersionInput.Value() != "1.2" {
		t.Fatalf("vpVersionInput = %q, want 1.2", got.vpVersionInput.Value())
	}
}

func TestVersionInputTabSwitchesToPortField(t *testing.T) {
	m := testModel()
	m.vpActive = true
	m.vpService = "SERVICE-A"
	m.vpVersionInput = newVPInput("version")
	m.vpVersionInput.SetValue("1.2.3")
	m.vpVersionInput.Focus()
	m.vpPortInput = newVPInput("port")
	m.vpPortInput.SetValue("8080")
	m.vpField = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := updated.(model)

	if got.vpField != 1 {
		t.Fatalf("vpField = %d, want 1 after Tab", got.vpField)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")})
	got = updated.(model)
	if got.vpPortInput.Value() != "80809" {
		t.Fatalf("vpPortInput = %q, want 80809 after typing on port field", got.vpPortInput.Value())
	}
}

func TestVersionInputEscCancels(t *testing.T) {
	m := testModel()
	m.vpActive = true
	m.vpService = "SERVICE-A"
	m.vpVersionInput = newVPInput("version")
	m.vpVersionInput.SetValue("1.2")
	m.vpPortInput = newVPInput("port")
	m.vpPortInput.SetValue("8080")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(model)

	if got.vpActive {
		t.Fatal("expected vpActive to be false after Esc")
	}
	if got.vpService != "" {
		t.Fatalf("expected vpService to be cleared after Esc, got %q", got.vpService)
	}
}

func TestVersionInputEnterStartsService(t *testing.T) {
	m := testModel()
	m.vpActive = true
	m.vpService = "SERVICE-A"
	m.vpVersionInput = newVPInput("version")
	m.vpVersionInput.SetValue("1.2.3")
	m.vpVersionInput.Focus()
	m.vpPortInput = newVPInput("port")
	m.vpPortInput.SetValue("9000")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("expected start command after Enter")
	}
	if got.vpActive {
		t.Fatal("expected vpActive to be false after Enter")
	}
	if got.popup.kind != actionOutputPopup {
		t.Fatalf("popup kind = %d, want actionOutputPopup", got.popup.kind)
	}
	if !strings.Contains(got.popup.title, "SERVICE-A:1.2.3") {
		t.Fatalf("popup title = %q, want SERVICE-A:1.2.3", got.popup.title)
	}
}

func TestCapitalRKeyEntersRestartInputMode(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	got := updated.(model)

	if !got.vpActive {
		t.Fatal("expected vpActive to be true after R")
	}
	if !got.vpRestart {
		t.Fatal("expected vpRestart to be true after R")
	}
	if got.vpService != "SERVICE-A" {
		t.Fatalf("vpService = %q, want SERVICE-A", got.vpService)
	}
	if got.vpVersionInput.Value() != "1.0.0" {
		t.Fatalf("vpVersionInput = %q, want 1.0.0 (pre-filled from service)", got.vpVersionInput.Value())
	}
	if got.vpPortInput.Value() != "8001" {
		t.Fatalf("vpPortInput = %q, want 8001 (pre-filled from service)", got.vpPortInput.Value())
	}
}

func TestVersionInputEnterRestartsServiceWhenVpRestart(t *testing.T) {
	m := testModel()
	m.vpActive = true
	m.vpRestart = true
	m.vpService = "SERVICE-A"
	m.vpVersionInput = newVPInput("version")
	m.vpVersionInput.SetValue("1.2.3")
	m.vpVersionInput.Focus()
	m.vpPortInput = newVPInput("port")
	m.vpPortInput.SetValue("9000")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("expected restart command after Enter")
	}
	if got.vpActive {
		t.Fatal("expected vpActive to be false after Enter")
	}
	if got.vpRestart {
		t.Fatal("expected vpRestart to be false after Enter")
	}
	if got.popup.kind != actionOutputPopup {
		t.Fatalf("popup kind = %d, want actionOutputPopup", got.popup.kind)
	}
	if !strings.Contains(got.popup.title, "Restarting") {
		t.Fatalf("popup title = %q, want Restarting prefix", got.popup.title)
	}
	if !strings.Contains(got.popup.title, "SERVICE-A:1.2.3") {
		t.Fatalf("popup title = %q, want SERVICE-A:1.2.3", got.popup.title)
	}
}

// Mouse tests use a 100×30 terminal. Panel geometry for that size:
//   leftX=2  rightBound=32
//   profiles  top=10 height=5  (rows 10–14)
//   running   top=15 height=5  (rows 15–19)
//   stopped   top=20 height=7  (rows 20–26)
// Row 0 of a panel is its top border; content starts at row 1.
// Running services in sample data: SERVICE-A (idx 0), SERVICE-C (idx 1).

func TestMouseClickActivatesPanel(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	// Start on profiles panel; click running panel's top border.
	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 10, Y: 15})
	got := updated.(model)

	if got.activePanel != runningPanel {
		t.Fatalf("activePanel = %d, want runningPanel after clicking running panel border", got.activePanel)
	}
}

func TestMouseClickMovesCursorToRow(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m.activePanel = runningPanel

	// y=16 is row 1 inside running panel → item index 0 (SERVICE-A).
	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 10, Y: 16})
	got := updated.(model)
	if got.cursors[runningPanel] != 0 {
		t.Fatalf("cursor = %d, want 0 (SERVICE-A) after clicking row 1", got.cursors[runningPanel])
	}

	// y=17 is row 2 → item index 1 (SERVICE-C).
	updated, _ = got.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 10, Y: 17})
	got = updated.(model)
	if got.cursors[runningPanel] != 1 {
		t.Fatalf("cursor = %d, want 1 (SERVICE-C) after clicking row 2", got.cursors[runningPanel])
	}
}

func TestMouseScrollDownMovesCursorDown(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m.activePanel = runningPanel
	m.cursors[runningPanel] = 0

	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseWheelDown, X: 10, Y: 16})
	got := updated.(model)

	if got.cursors[runningPanel] != 1 {
		t.Fatalf("cursor = %d, want 1 after scroll down", got.cursors[runningPanel])
	}
}

func TestMouseScrollUpMovesCursorUp(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m.activePanel = runningPanel
	m.cursors[runningPanel] = 1

	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseWheelUp, X: 10, Y: 16})
	got := updated.(model)

	if got.cursors[runningPanel] != 0 {
		t.Fatalf("cursor = %d, want 0 after scroll up", got.cursors[runningPanel])
	}
}

func TestMouseScrollActivatesPanelUnderCursor(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m.activePanel = profilesPanel // start on a different panel

	// Scroll over the running panel.
	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseWheelDown, X: 10, Y: 16})
	got := updated.(model)

	if got.activePanel != runningPanel {
		t.Fatalf("activePanel = %d, want runningPanel after scrolling over it", got.activePanel)
	}
}

func TestMouseClickOutsideLeftPanelsIgnored(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m.activePanel = runningPanel

	// x=50 is in the right panel (rightBound=32), should not change activePanel.
	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 50, Y: 16})
	got := updated.(model)

	if got.activePanel != runningPanel {
		t.Fatalf("activePanel = %d, want runningPanel (unchanged) after click on right panel", got.activePanel)
	}
}

// --- log follow-mode tests --------------------------------------------------

func TestLKeyEnablesLogFollowing(t *testing.T) {
	m := testModel()
	m.activePanel = runningPanel

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	got := updated.(model)

	if !got.logFollowing {
		t.Fatal("expected logFollowing = true after pressing l")
	}
	if got.selectedLog != "SERVICE-A" {
		t.Fatalf("selectedLog = %q, want SERVICE-A", got.selectedLog)
	}
}

func TestPgUpDisablesLogFollowing(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m = m.syncViewport()
	m.selectedLog = "SERVICE-A"
	m.logFollowing = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	got := updated.(model)

	if got.logFollowing {
		t.Fatal("expected logFollowing = false after PgUp")
	}
}

func TestCapitalGReenablesLogFollowing(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m = m.syncViewport()
	m.selectedLog = "SERVICE-A"
	m.logFollowing = false

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	got := updated.(model)

	if !got.logFollowing {
		t.Fatal("expected logFollowing = true after G")
	}
}

func TestNewLogLinesDoNotScrollWhenNotFollowing(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m = m.syncViewport()
	m.selectedLog = "SERVICE-A"
	m.logSession = 1
	m.logFollowing = false

	// Pre-populate with enough content that the viewport is not trivially at bottom.
	// Use the same mark format that the logReadMsg handler produces.
	mark := logMarkStyle.Render("│") + " "
	existing := strings.Repeat(mark+"old log line\n", 50)
	m.logContent = existing
	m.viewport.SetContent(existing)
	yBefore := m.viewport.YOffset

	updated, _ := m.Update(logReadMsg{
		serviceName: "SERVICE-A",
		session:     1,
		lines:       []string{"new line"},
		offset:      100,
		initialized: true,
	})
	got := updated.(model)

	if got.viewport.YOffset != yBefore {
		t.Fatalf("viewport scrolled to %d despite logFollowing=false (was %d)", got.viewport.YOffset, yBefore)
	}
}

func TestCapLines(t *testing.T) {
	cases := []struct {
		input string
		max   int
		want  string
	}{
		{"a\nb\nc\n", 3, "a\nb\nc\n"},
		{"a\nb\nc\n", 2, "b\nc\n"},
		{"a\nb\nc\n", 1, "c\n"},
		{"a\nb\nc\n", 10, "a\nb\nc\n"},
		{"", 5, ""},
	}
	for _, tc := range cases {
		got := capLines(tc.input, tc.max)
		if got != tc.want {
			t.Errorf("capLines(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
}

func TestLogPanelTitleShowsFollowingTag(t *testing.T) {
	m := testModel()
	m.width = 100
	m.height = 30
	m.selectedLog = "SERVICE-A"

	m.logFollowing = true
	view := m.logPanelView(60, 20)
	if !strings.Contains(view, "[following]") {
		t.Fatalf("log panel missing [following] tag:\n%s", view)
	}

	m.logFollowing = false
	view = m.logPanelView(60, 20)
	if !strings.Contains(view, "[paused") {
		t.Fatalf("log panel missing [paused] tag:\n%s", view)
	}
}

func TestTailFileReturnsLastNLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.log")
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	lines, _, _, err := readLogFile(path, 0, false)
	if err != nil {
		t.Fatalf("readLogFile: %v", err)
	}
	if len(lines) != initialLogLines {
		t.Fatalf("got %d lines, want %d", len(lines), initialLogLines)
	}
	// The last line in the file should be the last returned line.
	if lines[len(lines)-1] != "line 499" {
		t.Fatalf("last line = %q, want \"line 499\"", lines[len(lines)-1])
	}
}

func TestOccupiedPortsAreNotShownAsStoppedServices(t *testing.T) {
	m := model{
		status: sm2.Status{
			Services: []sm2.Service{
				{Name: "SERVICE-A", Version: "", PID: "0", Port: "8001", Status: "PASS", IsRunning: true},
				{Name: "SERVICE-B", Version: "1.0.0", PID: "10002", Port: "8002", Status: "PASS", IsRunning: true},
			},
			OccupiedPorts: []sm2.OccupiedPort{
				{PID: "10003", Port: "8003", ServiceName: "SERVICE-C"},
			},
		},
		cursors: make(map[panel]int),
	}

	m.updateServiceLists()

	if len(m.stoppedServices) != 0 {
		t.Fatalf("stopped services count = %d, want 0", len(m.stoppedServices))
	}
}
