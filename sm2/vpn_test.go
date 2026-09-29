package sm2

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckVPN(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	ok, err := checkVPN(&http.Client{}, svr.URL, 4*time.Second)
	if err != nil || !ok {
		t.Fatalf("checkVPN() = %v, %v; want true, nil", ok, err)
	}
}

func TestCheckVPNFailsOnNon200(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer svr.Close()

	ok, err := checkVPN(&http.Client{}, svr.URL, 4*time.Second)
	if err == nil || ok {
		t.Fatalf("checkVPN() = %v, %v; want false, error", ok, err)
	}
}

func TestCheckVPNFailsOnTimeout(t *testing.T) {
	ok, err := checkVPN(&http.Client{}, "http://254.254.254.254/ping", time.Millisecond)
	if err == nil || ok {
		t.Fatalf("checkVPN() = %v, %v; want false, error", ok, err)
	}
}

func TestPingURLFromConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	config := `{"artifactory": {"protocol": "https", "host": "example.com", "ping": "api/system/ping"}}`
	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, want := pingURLFromConfig(configFile), "https://example.com/api/system/ping"; got != want {
		t.Fatalf("pingURLFromConfig() = %q, want %q", got, want)
	}
}

func TestPingURLFromConfigFallsBackToDefault(t *testing.T) {
	if got := pingURLFromConfig(filepath.Join(t.TempDir(), "missing.json")); got != defaultArtifactoryPingURL {
		t.Fatalf("pingURLFromConfig() = %q, want %q", got, defaultArtifactoryPingURL)
	}
}

func TestPingURLFromConfigWithoutPingFallsBackToDefault(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{"artifactory": {"protocol": "https", "host": "example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := pingURLFromConfig(configFile); got != defaultArtifactoryPingURL {
		t.Fatalf("pingURLFromConfig() = %q, want %q", got, defaultArtifactoryPingURL)
	}
}
