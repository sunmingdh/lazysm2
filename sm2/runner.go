package sm2

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Service represents a single microservice managed by sm2.
type Service struct {
	Name      string
	Version   string
	PID       string
	Port      string
	Status    string
	IsRunning bool
}

// OccupiedPort represents a configured service port occupied by a process
// that is not managed by sm2.
type OccupiedPort struct {
	PID         string
	Port        string
	ServiceName string
}

// Status represents the overall status from the sm2 command.
type Status struct {
	Services        []Service
	OccupiedPorts   []OccupiedPort
	Profiles        []string
	ProfileServices map[string][]string
	ProfileLoadErr  error
}

var (
	// Compiled once at init time; used in the hot path of every status refresh.
	reStatusColumns = regexp.MustCompile(`\s*\|\s*`)
	reLogFilesDir   = regexp.MustCompile(`(?m)^Log files in (.+):\s*$`)

	// VPN interface name prefixes checked by isVPNConnected.
	vpnPrefixes = []string{
		"tun",   // OpenVPN, WireGuard (Linux)
		"tap",   // OpenVPN tap mode
		"ppp",   // PPTP, L2TP
		"wg",    // WireGuard
		"vpn",   // Generic VPN
		"ipsec", // IPSec
		"l2tp",  // L2TP
		"utun",  // macOS VPN (utun0, utun1, …)
	}
)

// GetStatus executes sm2 --status and sm2 --list concurrently and merges the results.
func GetStatus() (Status, error) {
	type profileResult struct {
		profiles        []string
		profileServices map[string][]string
		err             error
	}

	profileCh := make(chan profileResult, 1)
	go func() {
		profiles, profileServices, err := GetProfilesWithServices()
		profileCh <- profileResult{profiles, profileServices, err}
	}()

	status, err := getStatusOnly()
	if err != nil {
		return status, err
	}

	pr := <-profileCh
	if pr.err != nil {
		status.ProfileLoadErr = fmt.Errorf("load profiles: %w", pr.err)
		return status, nil
	}
	status.Profiles = pr.profiles
	status.ProfileServices = pr.profileServices
	return status, nil
}

func getStatusOnly() (Status, error) {
	cmd := exec.Command("sm2", "--status")
	output, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
			return Status{}, ee
		}
		return parseStatusOutput(string(output))
	}
	return parseStatusOutput(string(output))
}

// GetProfilesWithServices executes sm2 --list and parses profile names
// together with their contained services.
func GetProfilesWithServices() ([]string, map[string][]string, error) {
	cmd := exec.Command("sm2", "--list")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, err
	}
	profiles, profileServices := parseProfilesOutput(string(output))
	return profiles, profileServices, nil
}

func parseProfilesOutput(output string) ([]string, map[string][]string) {
	var profiles []string
	profileServices := make(map[string][]string)
	currentProfile := ""

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "[PROFILE]") {
			parts := strings.Fields(line)
			currentProfile = ""
			if len(parts) >= 2 {
				currentProfile = parts[1]
				profiles = append(profiles, currentProfile)
				if _, ok := profileServices[currentProfile]; !ok {
					profileServices[currentProfile] = []string{}
				}
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if currentProfile != "" && strings.HasPrefix(trimmed, "- ") {
			serviceName := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if serviceName != "" {
				profileServices[currentProfile] = append(profileServices[currentProfile], serviceName)
			}
			continue
		}

		if strings.HasPrefix(line, "[SERVICE]") {
			currentProfile = ""
		}
	}
	return profiles, profileServices
}

// IsOfflineMode reports whether services should be started with --offline,
// based on the START_SERVICE_OFFLINE env var and VPN connection status.
func IsOfflineMode() bool {
	return alwaysStartOffline() || !isVPNConnected()
}

// buildStartCmd constructs the sm2 --start command, adding --port and --offline when needed.
func buildStartCmd(name, port string) *exec.Cmd {
	args := []string{"--start", name}
	if port != "" {
		args = append(args, "--port", port)
	}
	if IsOfflineMode() {
		args = append(args, "--offline")
	}
	return exec.Command("sm2", args...)
}

// StartServiceOutput starts a service and returns its combined stdout/stderr output.
func StartServiceOutput(name string) (string, error) {
	output, err := buildStartCmd(name, "").CombinedOutput()
	return string(output), err
}

// StartServiceOutputStream starts a service and calls onLine for each output
// line as it is emitted. It returns the collected output.
func StartServiceOutputStream(name, port string, onLine func(string)) (string, error) {
	return runCommandStreamingOutput(buildStartCmd(name, port), onLine)
}

func runCommandStreamingOutput(cmd *exec.Cmd, onLine func(string)) (string, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var (
		mu       sync.Mutex
		output   strings.Builder
		wg       sync.WaitGroup
		scanErrs []error
	)

	scan := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			output.WriteString(line)
			output.WriteByte('\n')
			mu.Unlock()
			if onLine != nil {
				onLine(line)
			}
		}
		if err := scanner.Err(); err != nil {
			mu.Lock()
			scanErrs = append(scanErrs, err)
			mu.Unlock()
		}
	}

	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	wg.Wait()

	waitErr := cmd.Wait()

	mu.Lock()
	collected := output.String()
	var scanErr error
	if len(scanErrs) > 0 {
		scanErr = scanErrs[0]
	}
	mu.Unlock()
	if waitErr != nil {
		return collected, waitErr
	}
	return collected, scanErr
}

// isVPNConnected reports whether an active IPv4 VPN interface is present.
func isVPNConnected() bool {
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}

	for _, iface := range interfaces {
		name := strings.ToLower(iface.Name)
		for _, prefix := range vpnPrefixes {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			isUp := (iface.Flags & net.FlagUp) != 0
			isLoopback := (iface.Flags & net.FlagLoopback) != 0
			if !isUp || isLoopback {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && ip.To4() != nil {
					return true
				}
			}
		}
	}
	return false
}

// alwaysStartOffline reports whether START_SERVICE_OFFLINE is set to a truthy value.
// Recognised values: 1, true, yes, on, y (case-insensitive).
func alwaysStartOffline() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("START_SERVICE_OFFLINE"))) {
	case "1", "true", "yes", "on", "y":
		return true
	default:
		return false
	}
}

// StartMode returns a display string describing the current start mode.
func StartMode() string {
	if alwaysStartOffline() {
		return "OFFLINE Forced"
	}
	if isVPNConnected() {
		return "ONLINE"
	}
	return "OFFLINE no VPN"
}

// StopServiceOutput stops a service and returns its combined stdout/stderr output.
func StopServiceOutput(name string) (string, error) {
	output, err := exec.Command("sm2", "--stop", name).CombinedOutput()
	return string(output), err
}

// StopAllOutput stops all running services and returns the combined output.
func StopAllOutput() (string, error) {
	output, err := exec.Command("sm2", "--stop-all").CombinedOutput()
	return string(output), err
}

// DebugService executes sm2 --debug <service> and returns its output.
func DebugService(name string) (string, error) {
	output, err := exec.Command("sm2", "--debug", name).CombinedOutput()
	return string(output), err
}

// StdoutLogPathFromDebugOutput extracts the log directory from sm2 --debug output
// and returns the path to stdout.log inside it.
func StdoutLogPathFromDebugOutput(output string) (string, error) {
	matches := reLogFilesDir.FindStringSubmatch(output)
	if len(matches) != 2 {
		return "", fmt.Errorf("debug output did not include a log files directory")
	}
	return filepath.Join(strings.TrimSpace(matches[1]), "stdout.log"), nil
}

// parseStatusOutput parses the tabular output from sm2 --status.
func parseStatusOutput(output string) (Status, error) {
	var status Status
	inServicesTable := false
	inOccupiedPortsTable := false

	for _, line := range strings.Split(output, "\n") {
		trimmedLine := strings.TrimSpace(line)

		if strings.Contains(line, "Name") && strings.Contains(line, "Version") {
			inServicesTable = true
			inOccupiedPortsTable = false
			continue
		} else if strings.Contains(line, "PID") && strings.Contains(line, "Reserved by") {
			inServicesTable = false
			inOccupiedPortsTable = true
			continue
		}

		if !strings.HasPrefix(trimmedLine, "|") || !strings.HasSuffix(trimmedLine, "|") {
			continue
		}

		columns := reStatusColumns.Split(trimmedLine[1:len(trimmedLine)-1], -1)
		for i := range columns {
			columns[i] = strings.TrimSpace(columns[i])
		}

		if inServicesTable && len(columns) == 5 {
			name := columns[0]
			if name == "Name" || name == "" {
				continue
			}
			// Continuation line: sm2 wraps long names onto a second row when the
			// terminal is narrow. The continuation row has only the name fragment;
			// all other columns are empty.
			if columns[1] == "" && columns[2] == "" && columns[3] == "" && columns[4] == "" && len(status.Services) > 0 {
				status.Services[len(status.Services)-1].Name += name
				continue
			}
			status.Services = append(status.Services, Service{
				Name:      name,
				Version:   columns[1],
				PID:       columns[2],
				Port:      columns[3],
				Status:    columns[4],
				IsRunning: strings.Contains(columns[4], "PASS"),
			})
		} else if inOccupiedPortsTable && len(columns) == 3 {
			pid := columns[0]
			if pid == "PID" || pid == "" {
				continue
			}
			status.OccupiedPorts = append(status.OccupiedPorts, OccupiedPort{
				PID:         pid,
				Port:        columns[1],
				ServiceName: columns[2],
			})
		}
	}

	return status, nil
}
