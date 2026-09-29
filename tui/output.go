package tui

import (
	"fmt"
	"strconv"
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

// cleanCommandOutput cleans each line and replays cursor-up redraws (as used by
// sm2's progress bars) so each redrawn frame replaces the previous one.
func cleanCommandOutput(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		up := min(cursorUpCount(line), len(lines))
		lines = append(lines[:len(lines)-up], cleanCommandOutputLine(line))
	}
	return strings.Join(lines, "\n")
}

// cursorUpCount returns how many lines the leading escape sequences of line
// move the cursor up. sm2 redraws its progress bars by prefixing each frame
// with "\033[F\033[2K\r" once per previously drawn line.
func cursorUpCount(line string) int {
	up := 0
	for len(line) > 0 {
		if line[0] == '\r' {
			line = line[1:]
			continue
		}
		if !strings.HasPrefix(line, "\x1b[") {
			break
		}
		// CSI sequence: parameter bytes then a final byte in '@'..'~'.
		end := 2
		for end < len(line) && (line[end] < '@' || line[end] > '~') {
			end++
		}
		if end == len(line) {
			break
		}
		if final := line[end]; final == 'A' || final == 'F' {
			n, err := strconv.Atoi(line[2:end])
			if err != nil || n < 1 {
				n = 1
			}
			up += n
		}
		line = line[end+1:]
	}
	return up
}

// dropLastLines removes the last n newline-separated lines from s.
func dropLastLines(s string, n int) string {
	for ; n > 0; n-- {
		idx := strings.LastIndexByte(s, '\n')
		if idx < 0 {
			return ""
		}
		s = s[:idx]
	}
	return s
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
