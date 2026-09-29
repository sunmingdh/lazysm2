package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"lazysm2/tui"
)

// Set by GoReleaser's default ldflags (-X main.version=... -X main.commit=...).
var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(versionString())
		return
	}
	tui.Version = displayVersion()

	debugLog := len(os.Args) > 1 && os.Args[1] == "--debug-log"

	var logFile *os.File
	if debugLog {
		var err error
		logFile, err = setupLogging()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error setting up logging: %v\n", err)
			os.Exit(1)
		}
		defer logFile.Close()
	} else {
		log.SetOutput(io.Discard)
	}

	if err := tui.StartUIWithFallback(); err != nil {
		log.Fatalf("Error running UI: %v", err)
	}
	fmt.Println("lazysm2 has shut down.")
}

// displayVersion returns the release version as "vX.Y.Z", falling back to the
// module version for binaries built with `go install ...@vX.Y.Z`, or "dev".
func displayVersion() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}
	if v == "dev" {
		return v
	}
	return "v" + strings.TrimPrefix(v, "v")
}

// versionString is the --version output, e.g. "lazysm2 v0.1.3 (00a9d67)".
func versionString() string {
	v, c := displayVersion(), commit
	if c == "none" {
		return "lazysm2 " + v
	}
	if len(c) > 7 {
		c = c[:7]
	}
	return fmt.Sprintf("lazysm2 %s (%s)", v, c)
}

func setupLogging() (*os.File, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = "."
	}
	dir := cacheDir + "/lazysm2"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(dir+"/lazysm2.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return logFile, nil
}
