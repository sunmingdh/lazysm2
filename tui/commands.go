package tui

import (
	"fmt"
	"strings"
	"time"

	"lazysm2/sm2"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	logPollInterval = 500 * time.Millisecond
	initialLogLines = 200
	maxLogLines     = 5000 // maximum lines retained in the log panel
	vpnPollInterval = 30 * time.Second
)

// vpnCheckCommand runs the (potentially slow) VPN check off the UI goroutine.
func vpnCheckCommand(client sm2Client) tea.Cmd {
	return func() tea.Msg {
		client.RefreshVPNStatus()
		return vpnCheckedMsg{}
	}
}

// scheduleVPNCheck re-runs the VPN check after vpnPollInterval.
func scheduleVPNCheck(client sm2Client) tea.Cmd {
	return tea.Tick(vpnPollInterval, func(time.Time) tea.Msg {
		return vpnCheckCommand(client)()
	})
}

func refreshStatusCommand(client sm2Client) tea.Cmd {
	return func() tea.Msg {
		status, err := client.GetStatus()
		if err != nil {
			return errorMessage{err}
		}
		warning := ""
		if status.ProfileLoadErr != nil {
			warning = status.ProfileLoadErr.Error()
		}
		return statusRefreshedMsg{status: status, warning: warning}
	}
}

func startServiceCmd(client sm2Client, session int, serviceName string) tea.Cmd {
	return func() tea.Msg {
		return startServiceStream(session, serviceName, "Starting", "started", cleanCommandOutputLine, func(onLine func(string)) (string, error) {
			return client.StartServiceOutputStream(serviceName, "", onLine)
		})
	}
}

func startProfileCmd(client sm2Client, session int, profileName string, services []string) tea.Cmd {
	return func() tea.Msg {
		if len(services) == 0 {
			return actionOutputMsg{
				session:  session,
				name:     profileName,
				verb:     "Starting",
				pastVerb: "started",
				output:   "No services found in profile.",
				err:      nil,
			}
		}

		return startServiceStream(session, profileName, "Starting", "started", cleanCommandOutputLine, func(onLine func(string)) (string, error) {
			var combinedOutput string
			var hasErr error
			for _, serviceName := range services {
				header := fmt.Sprintf("=== Starting %s ===", serviceName)
				onLine(header)
				output, err := client.StartServiceOutputStream(serviceName, "", onLine)
				combinedOutput += fmt.Sprintf("%s\n%s\n", header, output)
				if err != nil {
					hasErr = err
				}
			}
			return combinedOutput, hasErr
		})
	}
}

func startServiceStream(session int, name, verb, pastVerb string, formatLine func(string) string, run func(func(string)) (string, error)) tea.Msg {
	lines := make(chan streamLine)
	done := make(chan actionOutputMsg, 1)
	go func() {
		output, err := run(func(line string) {
			// Count cursor-up redraws before formatLine strips the escape codes.
			up := cursorUpCount(line)
			if formatLine != nil {
				line = formatLine(line)
			}
			lines <- streamLine{text: line, up: up}
		})
		close(lines)
		done <- actionOutputMsg{
			session:  session,
			name:     name,
			verb:     verb,
			pastVerb: pastVerb,
			output:   output,
			err:      err,
		}
	}()
	return actionStreamStartedMsg{
		session:  session,
		name:     name,
		verb:     verb,
		pastVerb: pastVerb,
		lines:    lines,
		done:     done,
	}
}

func waitForActionLine(session int, name, verb string, lines <-chan streamLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-lines
		if !ok {
			return actionStreamClosedMsg{session: session}
		}
		return actionStreamLineMsg{session: session, name: name, verb: verb, line: line.text, up: line.up, lines: lines}
	}
}

func waitForActionDone(session int, done <-chan actionOutputMsg) tea.Cmd {
	return func() tea.Msg {
		msg := <-done
		msg.session = session
		return msg
	}
}

func stopServiceCmd(client sm2Client, serviceName string) tea.Cmd {
	return func() tea.Msg {
		output, err := client.StopServiceOutput(serviceName)
		return actionOutputMsg{
			name:     serviceName,
			verb:     "Stopping",
			pastVerb: "stopped",
			output:   output,
			err:      err,
		}
	}
}

func stopAllCmd(client sm2Client) tea.Cmd {
	return func() tea.Msg {
		output, err := client.StopAllOutput()
		return actionOutputMsg{
			name:     "all services",
			verb:     "Stopping",
			pastVerb: "stopped",
			output:   output,
			err:      err,
		}
	}
}

func startLogCmd(client sm2Client, serviceName string) tea.Cmd {
	return func() tea.Msg {
		output, err := client.DebugService(serviceName)
		path, pathErr := sm2.StdoutLogPathFromDebugOutput(output)
		if pathErr != nil {
			if err != nil {
				pathErr = fmt.Errorf("%w; debug command error: %v", pathErr, err)
			}
			return logStartMsg{serviceName: serviceName, output: output, err: pathErr}
		}
		return logStartMsg{serviceName: serviceName, path: path, output: output}
	}
}

func waitForLogFile(serviceName string, session int, path string, offset int64, initialized bool) tea.Cmd {
	return func() tea.Msg {
		lines, nextOffset, nextInitialized, err := readLogFile(path, offset, initialized)
		if err != nil || len(lines) == 0 {
			time.Sleep(logPollInterval)
		}
		return logReadMsg{
			serviceName: serviceName,
			session:     session,
			lines:       lines,
			offset:      nextOffset,
			initialized: nextInitialized,
			err:         err,
		}
	}
}

func debugServiceCmd(client sm2Client, serviceName string) tea.Cmd {
	return func() tea.Msg {
		output, err := client.DebugService(serviceName)
		return debugOutputMsg{
			serviceName: serviceName,
			output:      output,
			err:         err,
		}
	}
}

// restartServiceCmd stops then starts a service, streaming all output into a
// single popup so the user can follow both phases in one place.
func restartServiceCmd(client sm2Client, session int, serviceName string) tea.Cmd {
	return func() tea.Msg {
		return startServiceStream(session, serviceName, "Restarting", "restarted", cleanCommandOutputLine,
			func(onLine func(string)) (string, error) {
				onLine(fmt.Sprintf("=== Stopping %s ===", serviceName))
				stopOut, stopErr := client.StopServiceOutput(serviceName)
				for _, line := range strings.Split(stopOut, "\n") {
					if strings.TrimSpace(line) != "" {
						onLine(line)
					}
				}
				if stopErr != nil {
					onLine(fmt.Sprintf("Stop warning: %s", stopErr))
				}
				onLine(fmt.Sprintf("=== Starting %s ===", serviceName))
				return client.StartServiceOutputStream(serviceName, "", onLine)
			})
	}
}

// restartServiceWithVersionAndPortCmd stops a service then starts it at a
// specific version and optional port, streaming all output into a single popup.
// If version is empty the latest version is used; if port is empty the default
// port is used.
func restartServiceWithVersionAndPortCmd(client sm2Client, session int, serviceName, version, port string) tea.Cmd {
	target := serviceName
	if version != "" {
		target = serviceName + ":" + version
	}
	return func() tea.Msg {
		return startServiceStream(session, target, "Restarting", "restarted", cleanCommandOutputLine,
			func(onLine func(string)) (string, error) {
				onLine(fmt.Sprintf("=== Stopping %s ===", serviceName))
				stopOut, stopErr := client.StopServiceOutput(serviceName)
				for _, line := range strings.Split(stopOut, "\n") {
					if strings.TrimSpace(line) != "" {
						onLine(line)
					}
				}
				if stopErr != nil {
					onLine(fmt.Sprintf("Stop warning: %s", stopErr))
				}
				onLine(fmt.Sprintf("=== Starting %s ===", target))
				return client.StartServiceOutputStream(target, port, onLine)
			})
	}
}

// startServiceWithVersionAndPortCmd starts a service at a specific version and
// optional port using sm2 --start NAME[:VERSION] [--port PORT].
// If version is empty the latest version is used (no :VERSION suffix).
// If port is empty the default port is used (no --port flag).
func startServiceWithVersionAndPortCmd(client sm2Client, session int, serviceName, version, port string) tea.Cmd {
	target := serviceName
	if version != "" {
		target = serviceName + ":" + version
	}
	return func() tea.Msg {
		return startServiceStream(session, target, "Starting", "started", cleanCommandOutputLine,
			func(onLine func(string)) (string, error) {
				return client.StartServiceOutputStream(target, port, onLine)
			})
	}
}
