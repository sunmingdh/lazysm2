package tui

import (
	"io"
	"os"
	"strings"
)

func readLogFile(path string, offset int64, initialized bool) ([]string, int64, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, offset, initialized, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, offset, initialized, err
	}

	if !initialized {
		lines, err := tailFile(file, info.Size(), initialLogLines)
		if err != nil {
			return nil, offset, initialized, err
		}
		return lines, info.Size(), true, nil
	}

	if info.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, initialized, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, offset, initialized, err
	}
	return splitLogLines(string(data)), info.Size(), initialized, nil
}

// tailFile reads up to maxLines lines from the end of a file without loading
// the whole file into memory. It reads backward in doubling chunks until it has
// collected enough newline-separated lines.
func tailFile(file *os.File, size int64, maxLines int) ([]string, error) {
	if size == 0 {
		return nil, nil
	}

	// Start with an estimate of ~200 bytes per line; double until enough lines.
	readSize := int64(maxLines) * 200
	if readSize > size {
		readSize = size
	}

	for {
		startPos := size - readSize
		buf := make([]byte, readSize)
		if _, err := file.ReadAt(buf, startPos); err != nil {
			return nil, err
		}
		lines := splitLogLines(string(buf))
		// If we read the entire file, or collected more than enough lines, done.
		if startPos == 0 || len(lines) > maxLines {
			if len(lines) > maxLines {
				// The first element may be a partial line (we started mid-line);
				// taking the tail gives exactly maxLines complete lines.
				lines = lines[len(lines)-maxLines:]
			}
			return lines, nil
		}
		// Not enough lines yet — double the read window, capped at the full file.
		readSize = min(readSize*2, size)
	}
}

// splitLogLines splits newline-delimited text into lines, dropping a trailing empty line.
func splitLogLines(data string) []string {
	data = strings.TrimRight(data, "\n")
	if data == "" {
		return nil
	}
	return strings.Split(data, "\n")
}
