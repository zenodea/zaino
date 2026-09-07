package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const gitStatusLines = 20

func gitContext(cwd string) string {
	git := func(args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = cwd
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	if git("rev-parse", "--is-inside-work-tree") != "true" {
		return ""
	}

	branch := git("branch", "--show-current")
	if branch == "" {
		branch = git("rev-parse", "--short", "HEAD") + " (detached)"
	}
	if counts := strings.Fields(git("rev-list", "--left-right", "--count", "@{upstream}...HEAD")); len(counts) == 2 {
		branch += fmt.Sprintf(" · %s ahead, %s behind upstream", counts[1], counts[0])
	}

	lines := []string{"## git", "branch: " + branch}

	if status := git("status", "--short"); status == "" {
		lines = append(lines, "status: clean")
	} else {
		changed := strings.Split(status, "\n")
		lines = append(lines, fmt.Sprintf("status: %d paths changed", len(changed)))
		shown := changed
		if len(shown) > gitStatusLines {
			shown = shown[:gitStatusLines]
		}
		for _, l := range shown {
			lines = append(lines, "  "+l)
		}
		if len(changed) > len(shown) {
			lines = append(lines, fmt.Sprintf("  … and %d more", len(changed)-len(shown)))
		}
	}

	if log := git("log", "--oneline", "-5"); log != "" {
		lines = append(lines, "recent commits:")
		for _, l := range strings.Split(log, "\n") {
			lines = append(lines, "  "+l)
		}
	}
	return strings.Join(lines, "\n")
}

func ground(context string, withGit bool, cwd string) string {
	parts := []string{}
	if context != "" {
		parts = append(parts, context)
	}
	if withGit {
		if g := gitContext(cwd); g != "" {
			parts = append(parts, g)
		}
	}
	return strings.Join(parts, "\n\n")
}
