package tool

import (
	"fmt"
	"strings"
)

const (
	maxFileBytes  = 256 << 10
	maxReadLines  = 2000
	maxLineRunes  = 2000
	maxListed     = 500
	maxMatches    = 200
	maxOutputRune = 30000
	maxPreview    = 40
)

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"target": true, "dist": true, "build": true, ".venv": true,
}

func clip(s string, limit int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	return string(runes[:limit]), true
}

func clipOutput(s string) string {
	out, truncated := clip(s, maxOutputRune)
	if truncated {
		out += fmt.Sprintf("\n… truncated at %d characters", maxOutputRune)
	}
	return out
}

func clipLines(lines []string, limit int, what string) string {
	if len(lines) <= limit {
		return strings.Join(lines, "\n")
	}
	kept := strings.Join(lines[:limit], "\n")
	return fmt.Sprintf("%s\n… %d more %s", kept, len(lines)-limit, what)
}

func isBinary(b []byte) bool {
	head := b
	if len(head) > 8<<10 {
		head = head[:8<<10]
	}
	for _, c := range head {
		if c == 0 {
			return true
		}
	}
	return false
}
