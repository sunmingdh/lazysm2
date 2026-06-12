package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"lazysm2/tui"
)

func main() {
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
