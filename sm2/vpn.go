package sm2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// The VPN check mirrors sm2's own logic (servicemanager/vpn.go): the VPN is
// considered connected when the artifactory ping URL responds with HTTP 200
// within the short timeout.
const (
	defaultArtifactoryPingURL = "https://artefacts.tax.service.gov.uk/artifactory/api/system/ping"
	defaultWorkspaceDir       = ".sm2"
	defaultShortTimeout       = 5 * time.Second
)

// vpnState caches the result of the last VPN check so the UI can read it
// without blocking on the network.
var vpnState struct {
	mu        sync.Mutex
	checked   bool
	connected bool
}

// checkVPN tests VPN connectivity by requesting the artifactory ping URL
// with a short timeout.
func checkVPN(client *http.Client, pingURL string, timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pingURL, nil)
	if err != nil {
		return false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if _, err := io.ReadAll(resp.Body); err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("vpn check failed, http status %d", resp.StatusCode)
	}
	return true, nil
}

// RefreshVPNStatus runs the VPN check (blocking) and caches the result.
// It is skipped when START_SERVICE_OFFLINE forces offline starts.
func RefreshVPNStatus() bool {
	if alwaysStartOffline() {
		return false
	}
	connected, _ := checkVPN(http.DefaultClient, artifactoryPingURL(), vpnCheckTimeout())

	vpnState.mu.Lock()
	vpnState.checked = true
	vpnState.connected = connected
	vpnState.mu.Unlock()
	return connected
}

// cachedVPNStatus returns the last VPN check result and whether a check has completed.
func cachedVPNStatus() (connected, checked bool) {
	vpnState.mu.Lock()
	defer vpnState.mu.Unlock()
	return vpnState.connected, vpnState.checked
}

// isVPNConnected returns the cached VPN status, running a check first if none
// has completed yet.
func isVPNConnected() bool {
	if connected, checked := cachedVPNStatus(); checked {
		return connected
	}
	return RefreshVPNStatus()
}

// vpnCheckTimeout returns the VPN check timeout (shorter than sm2's 20s default
// since the check is polled), overridable via SM_TIMEOUT (seconds).
func vpnCheckTimeout() time.Duration {
	if value, err := strconv.ParseInt(os.Getenv("SM_TIMEOUT"), 10, 64); err == nil {
		return time.Duration(value) * time.Second
	}
	return defaultShortTimeout
}

// artifactoryPingURL resolves the ping URL the same way sm2 does: from
// $WORKSPACE/service-manager-config/config.json, falling back to the default.
func artifactoryPingURL() string {
	workspace, ok := os.LookupEnv("WORKSPACE")
	if !ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return defaultArtifactoryPingURL
		}
		workspace = filepath.Join(home, defaultWorkspaceDir)
	}
	return pingURLFromConfig(filepath.Join(workspace, "service-manager-config", "config.json"))
}

func pingURLFromConfig(configFile string) string {
	file, err := os.Open(configFile)
	if err != nil {
		return defaultArtifactoryPingURL
	}
	defer file.Close()

	var config struct {
		Artifactory struct {
			Protocol string `json:"protocol"`
			Host     string `json:"host"`
			Ping     string `json:"ping"`
		} `json:"artifactory"`
	}
	if err := json.NewDecoder(file).Decode(&config); err != nil || config.Artifactory.Ping == "" {
		return defaultArtifactoryPingURL
	}
	a := config.Artifactory
	return fmt.Sprintf("%s://%s/%s", a.Protocol, a.Host, a.Ping)
}
