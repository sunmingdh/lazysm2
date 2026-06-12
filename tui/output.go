package tui

import (
	"fmt"
	"strings"
)

func cleanDebugOutput(output string) string {
	output = strings.TrimSpace(output)
	lines := strings.Split(output, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "Checking .install file..." {
		lines = lines[1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func actionOutputContent(msg actionOutputMsg) string {
	output := strings.TrimSpace(cleanCommandOutput(msg.output))
	if output == "" {
		output = "No output."
	}
	if msg.err != nil {
		return fmt.Sprintf("%s\n\nError: %s", output, msg.err)
	}
	return output
}

func cleanCommandOutput(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lines[i] = cleanCommandOutputLine(line)
	}
	return strings.Join(lines, "\n")
}

// capLines returns s with all but the last max newline-terminated lines removed.
// If s has max or fewer lines it is returned unchanged.
func capLines(s string, max int) string {
	n := strings.Count(s, "\n")
	if n <= max {
		return s
	}
	skip := n - max
	pos := 0
	for i := 0; i < skip; i++ {
		idx := strings.IndexByte(s[pos:], '\n')
		if idx < 0 {
			break
		}
		pos += idx + 1
	}
	return s[pos:]
}

func cleanCommandOutputLine(line string) string {
	line = stripANSI(line)
	parts := strings.Split(line, "\r")
	line = parts[len(parts)-1]
	return strings.Map(func(r rune) rune {
		switch r {
		case '\t':
			return ' '
		case '\b':
			return -1
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, line)
}
