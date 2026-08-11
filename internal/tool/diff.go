package tool

import (
	"fmt"
	"strings"
)

func diffPreview(before, after string) string {
	old := splitLines(before)
	new := splitLines(after)

	head := 0
	for head < len(old) && head < len(new) && old[head] == new[head] {
		head++
	}
	tail := 0
	for tail < len(old)-head && tail < len(new)-head &&
		old[len(old)-1-tail] == new[len(new)-1-tail] {
		tail++
	}

	removed := old[head : len(old)-tail]
	added := new[head : len(new)-tail]
	if len(removed) == 0 && len(added) == 0 {
		return "no change"
	}

	lines := make([]string, 0, len(removed)+len(added))
	for i, line := range removed {
		lines = append(lines, fmt.Sprintf("%5d - %s", head+i+1, trim(line)))
	}
	for i, line := range added {
		lines = append(lines, fmt.Sprintf("%5d + %s", head+i+1, trim(line)))
	}
	return clipLines(lines, maxPreview, "changed lines")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func trim(line string) string {
	out, truncated := clip(line, 160)
	if truncated {
		out += " …"
	}
	return out
}
