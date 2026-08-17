package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zenodea/zaino/internal/agent"
)

// A command is a prompt you have written down: commands/review.md becomes
// /review, and what it says is sent as if you had typed it.
type Command struct {
	Name        string
	Description string
	Prompt      string
	Path        string

	// What to say instead of sending, when the model has not answered yet.
	// A command reaching back over the last answer is nonsense before there
	// is one; empty means the command does not care.
	Nothing string
}

// The commands zaino ships with are prompts like any other, so a file of your
// own by the same name replaces one outright.
func Builtins() []Command {
	return []Command{{
		Name:        "bro",
		Description: "say the last answer again, simply",
		Prompt:      broPrompt,
		Nothing:     "nothing to simplify yet, bro",
	}}
}

const broPrompt = `That last answer did not land — too dense, too much jargon, too formal.

Say the same thing again, simply, the way you would to a smart friend over a beer.

Re-explain, do not re-answer: nothing new, no tools, no fresh questions taken.
Simpler, not necessarily shorter — take the space the idea needs, and cut the
preamble, the hedging and the consultant-speak.
Every path, command, filename, number, URL, name and decision stays exactly as
it was; only the explanation around them gets easier.
Casual and direct — "ok so", "basically", "the point is" — with a little
personality, but do not turn it into a meme.
Answer in the language the last message was in.
Drop the headers and the ceremony: tables become sentences, and a list survives
only where the thing really did have parts.`

// $ARGUMENTS is everything after the command, $1 to $9 the words of it. A
// command that asks for neither still gets what you typed, on the end, rather
// than losing it.
func (c Command) Expand(arg string) string {
	arg = strings.TrimSpace(arg)
	prompt, used := c.Prompt, false

	if strings.Contains(prompt, "$ARGUMENTS") {
		prompt, used = strings.ReplaceAll(prompt, "$ARGUMENTS", arg), true
	}
	words := strings.Fields(arg)
	for i := 9; i >= 1; i-- {
		placeholder := fmt.Sprintf("$%d", i)
		if !strings.Contains(prompt, placeholder) {
			continue
		}
		word := ""
		if i <= len(words) {
			word = words[i-1]
		}
		prompt, used = strings.ReplaceAll(prompt, placeholder, word), true
	}

	if !used && arg != "" {
		return prompt + "\n\n" + arg
	}
	return prompt
}

func loadCommands(dir string) ([]Command, error) {
	files, err := markdownFiles(dir)
	if err != nil {
		return nil, err
	}

	out := make([]Command, 0, len(files))
	for _, path := range files {
		front, body, err := readMarkdown(path)
		if err != nil {
			return nil, err
		}
		out = append(out, Command{
			Name:        commandName(path),
			Description: front["description"],
			Prompt:      body,
			Path:        path,
		})
	}
	return out, nil
}

func loadSubagents(dir string) ([]agent.Subagent, error) {
	files, err := markdownFiles(dir)
	if err != nil {
		return nil, err
	}

	out := make([]agent.Subagent, 0, len(files))
	for _, path := range files {
		front, body, err := readMarkdown(path)
		if err != nil {
			return nil, err
		}
		out = append(out, agent.Subagent{
			Name:        commandName(path),
			Description: front["description"],
			Model:       front["model"],
			Tools:       commaList(front["tools"]),
			System:      body,
		})
	}
	return out, nil
}

func mergeCommands(base, over []Command) []Command {
	out := append([]Command(nil), base...)
	for _, c := range over {
		replaced := false
		for i := range out {
			if out[i].Name == c.Name {
				out[i], replaced = c, true
				break
			}
		}
		if !replaced {
			out = append(out, c)
		}
	}
	return out
}

func mergeSubagents(base, over []agent.Subagent) []agent.Subagent {
	out := append([]agent.Subagent(nil), base...)
	for _, a := range over {
		replaced := false
		for i := range out {
			if out[i].Name == a.Name {
				out[i], replaced = a, true
				break
			}
		}
		if !replaced {
			out = append(out, a)
		}
	}
	return out
}

// A missing directory is the normal case, not a failure.
func markdownFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

func commandName(path string) string {
	name := filepath.Base(path)
	return strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
}

// Front matter is the few keys between two rules at the top of the file:
//
//	---
//	description: read the diff and say what is wrong
//	tools: read, grep, find
//	---
func readMarkdown(path string) (map[string]string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	front := map[string]string{}

	if rest, ok := strings.CutPrefix(text, "---\n"); ok {
		head, body, closed := strings.Cut(rest, "\n---")
		if !closed {
			return nil, "", fmt.Errorf("%s: front matter is never closed", path)
		}
		for _, line := range strings.Split(head, "\n") {
			if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				return nil, "", fmt.Errorf("%s: %q is not key: value", path, line)
			}
			front[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
		text = body
	}

	return front, strings.TrimSpace(text), nil
}

func commaList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func findCommand(all []Command, name string) (Command, bool) {
	for _, c := range all {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}
