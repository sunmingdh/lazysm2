package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func helpContent() string {
	return "tab: switch panel\n" +
		"1/2/3: jump to panel\n" +
		"f: filter active panel\n" +
		"j/down: move down\n" +
		"k/up: move up\n" +
		"s: start service/profile\n" +
		"S: start service with specific version and port\n" +
		"r: restart service (stop then start)\n" +
		"R: restart service with specific version and port\n" +
		"x: stop service/profile\n" +
		"X: stop all services\n" +
		"l: view logs (running services)\n" +
		"PgUp/PgDn: scroll log/debug panel\n" +
		"G: jump to bottom / resume log follow\n" +
		"q: quit\n" +
		"?: toggle help\n\n" +
		helpStyle.Render("Shift+drag to copy text from the log panel.") + "\n" +
		helpStyle.Render("Press any key to close help.")
}

func clampMin(value, minValue int) int {
	if value < minValue {
		return minValue
	}
	return value
}

// scrollStart returns the index of the first visible item so the cursor
// always remains in view.
func scrollStart(cursor, visible int) int {
	if cursor < visible {
		return 0
	}
	return cursor - visible + 1
}

func sizedStyle(base lipgloss.Style, width, height int) lipgloss.Style {
	fw := base.GetHorizontalBorderSize()
	fh := base.GetVerticalBorderSize()
	return base.Copy().
		Width(clampMin(width-fw, 0)).
		Height(clampMin(height-fh, 0))
}

func sizedPanelStyle(width, height int) lipgloss.Style { return sizedStyle(panelStyle, width, height) }
func sizedPopupStyle(width, height int) lipgloss.Style { return sizedStyle(popupStyle, width, height) }

func panelContentHeight(height int) int {
	_, frameHeight := panelStyle.GetFrameSize()
	return clampMin(height-frameHeight, 0)
}

func popupContentHeight(height int) int {
	_, frameHeight := popupStyle.GetFrameSize()
	return clampMin(height-frameHeight, 0)
}

func joinPanelLines(lines []string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func panelContentWidth(width int) int {
	frameWidth, _ := panelStyle.GetFrameSize()
	return clampMin(width-frameWidth, 0)
}

func popupContentWidth(width int) int {
	frameWidth, _ := popupStyle.GetFrameSize()
	return clampMin(width-frameWidth, 0)
}

func truncateLines(lines []string, maxWidth int) []string {
	truncated := make([]string, 0, len(lines))
	for _, line := range lines {
		truncated = append(truncated, truncateText(line, maxWidth))
	}
	return truncated
}

func truncateText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= maxWidth {
		return text
	}
	var b strings.Builder
	current := 0
	for _, r := range text {
		rw := lipgloss.Width(string(r))
		if current+rw > maxWidth {
			break
		}
		b.WriteRune(r)
		current += rw
	}
	return b.String()
}

func stripANSI(text string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range text {
		if inEscape {
			if (r >= '@' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		if r == '\x1b' {
			inEscape = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func takeCells(text string, start, width int) string {
	if width <= 0 {
		return ""
	}

	var b strings.Builder
	current := 0
	for _, r := range text {
		cellWidth := lipgloss.Width(string(r))
		next := current + cellWidth
		if next > start && current < start+width {
			b.WriteRune(r)
		}
		current = next
		if current >= start+width {
			break
		}
	}
	return b.String()
}

func padLine(line string, width int) string {
	padding := width - lipgloss.Width(line)
	if padding <= 0 {
		return line
	}
	return line + strings.Repeat(" ", padding)
}

func overlayCentered(base, overlay string, width, height int) string {
	base = stripANSI(base)
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}

	overlayLines := strings.Split(overlay, "\n")
	overlayWidth := lipgloss.Width(overlay)
	overlayHeight := lipgloss.Height(overlay)
	x := clampMin((width-overlayWidth)/2, 0)
	y := clampMin((height-overlayHeight)/2, 0)

	for i, overlayLine := range overlayLines {
		target := y + i
		if target < 0 || target >= len(baseLines) {
			continue
		}
		lineWidth := lipgloss.Width(overlayLine)
		baseLine := padLine(baseLines[target], width)
		prefix := takeCells(baseLine, 0, x)
		suffix := takeCells(baseLine, x+lineWidth, width-x-lineWidth)
		baseLines[target] = prefix + overlayLine + suffix
	}

	return strings.Join(baseLines, "\n")
}

func titledTopBorder(title string, width int) string {
	if width <= 0 {
		return ""
	}
	if width == 1 {
		return "╭"
	}
	if width == 2 {
		return "╭╮"
	}

	innerWidth := width - 2
	title = truncateText(title, clampMin(innerWidth-2, 0))
	label := ""
	if title != "" {
		label = " " + titleStyle.Render(title) + " "
	}
	padding := strings.Repeat("─", clampMin(innerWidth-lipgloss.Width(label), 0))
	return "╭" + label + padding + "╮"
}

func renderTitledPanel(style lipgloss.Style, title, content string, width int) string {
	rendered := style.Render(content)
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}
	lines[0] = titledTopBorder(title, width)
	return strings.Join(lines, "\n")
}

func (m model) panelView(title string, items []string, p panel, width, height int) string {
	style := sizedPanelStyle(width, height)
	if m.activePanel == p {
		style = style.BorderForeground(lipgloss.Color("170"))
	}

	maxLines := panelContentHeight(height)
	lines := []string{}
	query := m.panelFilter(p)
	if m.filtering && m.activePanel == p || query != "" {
		prompt := "Filter: " + query
		if m.filtering && m.activePanel == p {
			prompt += "_"
		}
		lines = append(lines, helpStyle.Render(prompt))
	}
	if len(items) == 0 {
		message := "  None"
		if query != "" {
			message = "  No matches"
		}
		lines = append(lines, helpStyle.Render(message))
	} else {
		maxItems := clampMin(maxLines-len(lines), 0)
		start := scrollStart(m.cursors[p], maxItems)

		for i := start; i < len(items) && i < start+maxItems; i++ {
			cursor := " "
			if m.activePanel == p && m.cursors[p] == i {
				cursor = ">"
			}
			lines = append(lines, fmt.Sprintf("%s %s", cursor, items[i]))
		}
	}
	lines = truncateLines(lines, panelContentWidth(width))
	title = fmt.Sprintf("[%d] %s (%d)", int(p)+1, title, len(items))
	return renderTitledPanel(style, title, joinPanelLines(lines, maxLines), width)
}

func (m model) infoView(width, height int) string {
	var details string
	switch m.activePanel {
	case profilesPanel:
		profiles := m.filteredProfiles()
		if len(profiles) > 0 {
			details = fmt.Sprintf("Profile: %s", profiles[m.cursors[profilesPanel]])
		}
	case runningPanel:
		services := m.filteredRunningServices()
		if len(services) > 0 {
			s := services[m.cursors[runningPanel]]
			details = fmt.Sprintf("Name: %s\nVersion: %s\nPID: %s\nPort: %s\nStatus: %s", s.Name, s.Version, s.PID, s.Port, s.Status)
		}
	case stoppedPanel:
		services := m.filteredStoppedServices()
		if len(services) > 0 && m.cursors[stoppedPanel] < len(services) {
			s := services[m.cursors[stoppedPanel]]
			details = fmt.Sprintf("Name: %s\nVersion: %s\nPID: %s\nPort: %s\nStatus: %s", s.Name, s.Version, s.PID, s.Port, s.Status)
		}
	}

	if details == "" {
		details = "No item selected."
	}

	maxLines := panelContentHeight(height)
	lines := strings.Split(details, "\n")

	style := sizedPanelStyle(width, height).
		BorderForeground(lipgloss.Color("86"))
	return renderTitledPanel(style, "Info", joinPanelLines(lines, maxLines), width)
}

func (m model) logPanelView(width, height int) string {
	title := "Debug"
	if m.activePanel == profilesPanel && m.selectedLog == "" {
		return m.profileServicesPanelView(width, height)
	} else if m.selectedLog != "" {
		followTag := "[following]"
		if !m.logFollowing {
			followTag = "[paused - G to resume]"
		}
		title = fmt.Sprintf("Logs: %s %s", m.selectedLog, followTag)
	} else if m.selectedDebug != "" {
		title = fmt.Sprintf("Debug: %s", m.selectedDebug)
	}

	vp := m.viewport
	vp.Width = panelContentWidth(width)
	vp.Height = panelContentHeight(height)

	style := sizedPanelStyle(width, height)
	return renderTitledPanel(style, title, vp.View(), width)
}

func (m model) popupView() string {
	if m.popup.kind == noPopup {
		return ""
	}

	width := clampMin(m.width*3/5, 40)
	if m.width > 0 {
		width = min(width, clampMin(m.width-4, 1))
	}
	height := clampMin(m.height/2, 8)
	if m.height > 0 {
		height = min(height, clampMin(m.height-4, 1))
	}

	content := strings.TrimSpace(m.popup.content)
	if m.popup.kind == actionOutputPopup {
		closeHint := helpStyle.Render("Press esc or enter to close.")
		if content == "" {
			content = closeHint
		} else {
			content += "\n\n" + closeHint
		}
	}

	maxLines := popupContentHeight(height)
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	lines = truncateLines(lines, popupContentWidth(width))

	style := sizedPopupStyle(width, height)
	return renderTitledPanel(style, m.popup.title, joinPanelLines(lines, maxLines), width)
}

func (m model) profileServicesPanelView(width, height int) string {
	profileName := m.selectedProfileName()
	title := "Profile Services"
	lines := []string{}
	if profileName != "" {
		title = fmt.Sprintf("Profile: %s", profileName)
	}

	if profileName == "" {
		lines = append(lines, "No profile selected.")
	} else {
		services := m.profileServices[profileName]
		if len(services) == 0 {
			lines = append(lines, "No services found in this profile.")
		} else {
			running := m.runningServiceSet()
			for _, serviceName := range services {
				status := stoppedStyle.Render("stopped")
				rowStyle := stoppedStyle
				if _, ok := running[serviceName]; ok {
					status = runningStyle.Render("running")
					rowStyle = lipgloss.NewStyle()
				}
				lines = append(lines, rowStyle.Render(fmt.Sprintf("%s %s", status, serviceName)))
			}
		}
	}

	maxLines := panelContentHeight(height)
	lines = truncateLines(lines, panelContentWidth(width))

	style := sizedPanelStyle(width, height)
	return renderTitledPanel(style, title, joinPanelLines(lines, maxLines), width)
}

func (m model) baseView() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	hMargin, vMargin := docStyle.GetFrameSize()
	workingWidth := m.width - hMargin
	workingHeight := m.height - vMargin

	modeLabel := ""
	if m.sm2 != nil {
		modeLabel = m.sm2.StartMode()
	}
	modeBadge := ""
	if modeLabel != "" {
		modeBadge = " " + modeStyle.Render("["+modeLabel+"]")
	}

	header := titleStyle.Render("Service Manager") + modeBadge + " | " + m.statusMessage
	var footer string
	if m.vpActive {
		versionLabel := fmt.Sprintf("Version: %s", m.vpVersion)
		portLabel := fmt.Sprintf("  Port: %s", m.vpPort)
		if m.vpField == 0 {
			footer = titleStyle.Render(versionLabel+"█") + portLabel + helpStyle.Render("   Tab · Enter · Esc")
		} else {
			footer = versionLabel + titleStyle.Render(portLabel+"█") + helpStyle.Render("   Tab · Enter · Esc")
		}
	} else {
		footer = helpStyle.Render(m.footerHelp())
	}
	header = truncateText(header, workingWidth)
	footer = truncateText(footer, workingWidth)

	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)

	availableHeight := workingHeight - headerHeight - footerHeight - 2
	if availableHeight < 10 {
		return "Window too small"
	}

	leftWidth := workingWidth / 3
	rightWidth := workingWidth - leftWidth

	infoHeight := 7
	remainingHeight := availableHeight - infoHeight
	panelHeight := remainingHeight / 3
	lastPanelHeight := remainingHeight - (panelHeight * 2)

	info := m.infoView(leftWidth-2, infoHeight)
	profiles := m.panelView("Profiles", m.filteredProfiles(), profilesPanel, leftWidth-2, panelHeight)
	running := m.panelView("Running Services", m.filteredRunningServiceNames(), runningPanel, leftWidth-2, panelHeight)
	stopped := m.panelView("Stopped Services", m.filteredStoppedServiceNames(), stoppedPanel, leftWidth-2, lastPanelHeight)
	leftSide := lipgloss.JoinVertical(lipgloss.Left, info, profiles, running, stopped)

	logs := m.logPanelView(rightWidth-2, availableHeight)
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftSide, logs)

	finalView := lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		mainContent,
		"",
		footer,
	)

	return docStyle.Render(finalView)
}

func (m model) View() string {
	base := m.baseView()
	if m.popup.kind == noPopup {
		return base
	}
	return overlayCentered(base, m.popupView(), m.width, m.height)
}
