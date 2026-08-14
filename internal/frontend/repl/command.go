package repl

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/provider"
	"github.com/zenodea/zaino/internal/store/session"
)

// Prompts that merely open with a slash — "/etc/hosts is wrong" — are not
// commands.
func isCommand(line string) bool {
	if !strings.HasPrefix(line, "/") {
		return false
	}
	word, _, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	if word == "" {
		return false
	}
	return strings.IndexFunc(word, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_'
	}) < 0
}

const help = `/help                list the commands
/clear               forget the conversation  (/new, /reset)
/model [id]          show or change the model
/provider [name]     show or switch provider (clears the context)
/effort [level]      show or set output effort
/thinking [on|off]   show or hide the model's reasoning
/system [prompt|-]   show, set, or drop the system prompt
/permission [mode]   show or set when tools stop to ask  (/perm, /mode)
/tools               list the tools the model has
/compact             fold the conversation so far into a summary
/usage               token usage for this session
/sessions            list saved sessions      (/resume)
/quit                leave zaino              (/exit, /q)`

func runCommand(ag *agent.Agent, line string, messages []llm.Message,
	usage *llm.Usage, o Options) (cleared, quit bool) {
	notice := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "\x1b[2m"+format+"\x1b[0m\n", args...)
	}
	fail := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "\x1b[31m"+format+"\x1b[0m\n", args...)
	}

	name, arg, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	name, arg = strings.ToLower(name), strings.TrimSpace(arg)

	switch name {
	case "help", "h", "?":
		notice("%s", help)

	case "quit", "exit", "q":
		return false, true

	case "clear", "new", "reset":
		*usage = llm.Usage{}
		o.Recorder.Clear()
		notice("context cleared")
		return true, false

	case "model":
		if arg == "" {
			notice("model: %s", modelName(ag))
			if lister, ok := ag.Provider.(llm.ModelLister); ok {
				notice("known: %s", strings.Join(lister.Models(), ", "))
			}
			break
		}
		ag.Model = arg
		o.Recorder.Append(session.Model(ag.Provider.Name(), arg))
		notice("model → %s", arg)

	case "provider":
		if arg == "" {
			notice("provider: %s (available: %s)",
				ag.Provider.Name(), strings.Join(provider.Available(), ", "))
			break
		}
		backend, err := provider.New(arg)
		if err != nil {
			fail("%s", err)
			break
		}
		if backend.Name() == ag.Provider.Name() {
			notice("provider: already %s", backend.Name())
			break
		}
		// A model id, thinking signatures and tool ids mean nothing there.
		ag.Provider, ag.Model = backend, ""
		*usage = llm.Usage{}
		o.Recorder.Append(session.Model(backend.Name(), ""))
		o.Recorder.Clear()
		notice("provider → %s · %s (context cleared)", backend.Name(), backend.DefaultModel())
		return true, false

	case "effort":
		switch {
		case arg == "":
			notice("effort: %s (%s)", orNone(ag.Effort, "(provider default)"),
				strings.Join(efforts, ", "))
		case arg == "-" || arg == "default":
			ag.Effort = ""
			o.Recorder.Append(session.Effort(""))
			notice("effort → provider default")
		case slices.Contains(efforts, arg):
			ag.Effort = arg
			o.Recorder.Append(session.Effort(arg))
			notice("effort → %s", arg)
		default:
			fail("unknown effort %q — pick one of %s", arg, strings.Join(efforts, ", "))
		}

	case "thinking":
		if ag.Thinking == nil {
			ag.Thinking = &llm.Thinking{Enabled: true}
		}
		switch arg {
		case "":
			notice("thinking: %s", shownHidden(ag.Thinking.Show))
		case "on", "show", "true":
			ag.Thinking.Show = true
			o.Recorder.Append(session.Thinking(true))
			notice("thinking → shown")
		case "off", "hide", "false":
			ag.Thinking.Show = false
			o.Recorder.Append(session.Thinking(false))
			notice("thinking → hidden")
		default:
			fail("usage: /thinking [on|off], got %q", arg)
		}

	case "system":
		switch {
		case arg == "":
			notice("system: %s", orNone(ag.System, "(none)"))
		case arg == "-":
			ag.System = ""
			o.Recorder.Append(session.System(""))
			notice("system prompt dropped")
		default:
			ag.System = arg
			o.Recorder.Append(session.System(arg))
			notice("system prompt set (%d chars)", len(arg))
		}

	case "permission", "perm", "mode":
		gate := ag.Gate
		switch {
		case gate == nil || gate.Policy == nil:
			notice("permission: nothing is gated")
		case arg == "":
			notice("permission: %s — %s", gate.Mode(), gate.Mode().Describe())
			if granted := gate.Policy.Granted(); len(granted) > 0 {
				notice("allowed for this session: %s", strings.Join(granted, ", "))
			}
		default:
			mode, err := permission.ParseMode(arg)
			if err != nil {
				fail("%s", err)
				break
			}
			gate.Policy.SetMode(mode)
			notice("permission → %s (%s)", mode, mode.Describe())
		}

	case "tools":
		if len(ag.Tools) == 0 {
			notice("no tools — the model can only talk")
			break
		}
		lines := []string{fmt.Sprintf("tools · %s", ag.Gate.Mode())}
		for _, t := range ag.Tools {
			lines = append(lines, "  "+t.Definition().Name)
		}
		notice("%s", strings.Join(lines, "\n"))

	case "compact":
		folded, err := ag.Fold(context.Background(), messages)
		if err != nil {
			fail("%s", err)
			break
		}
		notice("compacted · %d messages kept", max(len(folded)-1, 0))
		return false, false

	case "usage":
		notice("usage · %s · %s\n  input     %d\n  output    %d\n  think     %d\n  cached    %d\n  messages  %d\n  session   %s",
			ag.Provider.Name(), modelName(ag), usage.InputTokens, usage.OutputTokens,
			usage.ThinkingTokens, usage.CacheReadTokens, len(messages), orNone(o.Recorder.ID(), "(not saved)"))

	case "sessions", "resume":
		if o.Repo == nil {
			notice("sessions are not being saved")
			break
		}
		list, err := o.Repo.List()
		if err != nil {
			fail("%s", err)
			break
		}
		if len(list) == 0 {
			notice("no saved sessions yet")
			break
		}
		lines := make([]string, 0, len(list)+1)
		lines = append(lines, "sessions · resume one with -r <id>")
		for _, s := range list {
			here := " "
			if s.ID == o.Recorder.ID() {
				here = "·"
			}
			lines = append(lines, fmt.Sprintf("%s %s  %3d msg  %s",
				here, s.ID, s.Messages, truncate(s.Preview, 48)))
		}
		notice("%s", strings.Join(lines, "\n"))

	default:
		fail("unknown command /%s — try /help", name)
	}
	return false, false
}

var efforts = []string{llm.EffortLow, llm.EffortMedium, llm.EffortHigh, llm.EffortXHigh, llm.EffortMax}

func modelName(ag *agent.Agent) string {
	if ag.Model != "" {
		return ag.Model
	}
	return ag.Provider.DefaultModel()
}

func shownHidden(v bool) string {
	if v {
		return "shown"
	}
	return "hidden"
}

func orNone(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:max(limit-1, 0)]) + "…"
}
