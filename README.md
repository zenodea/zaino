<p align="center">
  <img alt="zaino" src="assets/backpack.svg" width="128">
</p>
<p align="center">
  <a href="https://go.dev"><img alt="Go" src="https://img.shields.io/badge/go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white" /></a>
  <a href="LICENSE"><img alt="MIT" src="https://img.shields.io/badge/license-MIT-c4522a?style=flat-square" /></a>
</p>

# Zaino

A coding agent in your terminal, packed into one Go binary. *Zaino* is
Italian for backpack: the provider clients, the turn loop, the tools and the
UI are all written here, with three dependencies, all of them for drawing.

* **Five providers**, spoken over their raw HTTP APIs: Anthropic, Gemini, OpenAI, Grok, OpenRouter
* **Eight tools** — `read`, `write`, `edit`, `bash`, `grep`, `find`, `ls`, `fetch` — plus `task` for subagents and MCP servers over stdio
* **A full-screen UI** with a walkable transcript, vim editing, and a plain REPL for pipes
* **Sessions as a tree**: resume, `/rewind`, branch, and see the whole thing on a `/journey` map

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/zenodea/Zaino/main/install.sh | sh
```

Or from a checkout: `./install.sh`, or `go install ./cmd/zaino`. The binary
lands in `~/.local/bin`; `install.sh --uninstall` takes it away again.

## Running

Set a key for whichever provider you have, and zaino picks it up:

```bash
export ANTHROPIC_API_KEY=...     # or GEMINI_API_KEY, OPENAI_API_KEY, XAI_API_KEY, OPENROUTER_API_KEY

zaino                            # full-screen UI, provider auto-detected
zaino -provider gemini -model gemini-2.5-flash
zaino -c                         # continue the last session here
```

Anything piped in gets the line-based REPL, and `-p` asks once and leaves:

```bash
git diff | zaino -p "review this" -permission plan
zaino -p "what does install.sh do" -json | jq -r .answer
```

`zaino -h` lists the rest of the flags; every one of them can also be set
once in the config.

## What's in the pack

| Path | What it holds |
|------|---------------|
| `internal/llm/` | The provider-neutral model: messages, blocks, streams |
| `internal/provider/` | One client per provider, each knowing only its own wire format |
| `internal/agent/` | The turn loop — stream, run the tools, feed back, repeat — plus compaction and subagents |
| `internal/tool/` | The eight tools, and the rules that keep writes honest |
| `internal/permission/` | Who may do what, and when to ask |
| `internal/mcp/` | MCP servers, spawned on stdio |
| `internal/frontend/` | The Bubble Tea UI and the REPL |
| `internal/store/` | Sessions on disk, prompt recall, the wire log |
| `cmd/zaino/` | Flags, config and wiring |

Only `internal/provider/` knows a wire format. Adding a provider means
implementing one interface and nothing else.

## Permission

What the model may do without asking is a mode. `⇧⇥` cycles it, and
`-permission` or the config sets it:

```
manual         ask before writing, running or fetching   (the default)
accept-edits   edits go through, ask before running or fetching
plan           read only — nothing is written or run
bypass         everything goes through unasked
```

When zaino asks, `y` allows once, `a` allows that tool and target for the
session, `n` refuses — and a refusal is not an error, the model is told and
carries on. Standing answers live in the config as `allow` and `deny` rules.

One rule sits under all the modes: **paths outside the working directory are
refused, not asked about**, symlinks resolved first. `-allow-outside` is the
only thing that lifts it. A subagent inherits its parent's gate, so it is not
a way around a refusal.

## The UI

`⏎` sends, `⌥⏎` makes a newline, `↑`/`↓` recall earlier prompts. `⌃j`/`⌃k`
walk a bar through the transcript; `⏎` on a tool call opens it up. A running
turn does not lock the composer — type, and what you said goes in with the
next tool results. `esc` stops a turn; `⌃c` twice quits.

Vim editing is on by default (`-vim=false` turns it off): the usual motions,
counts, `dd`/`cw`/`x`, visual mode, `u` and `p`. The composer starts in
insert, so nothing changes until you press `esc`.

`@path` attaches a file — text goes in fenced, PNG/JPEG/GIF/WebP go in as
pictures.

## Commands

Typing `/` opens a fuzzy menu. The ones that only look — `/usage`, `/agents`,
`/tools`, `/help`, `/config` — work mid-turn; the rest wait.

```
/model /provider /effort /thinking /system /profile    the model-facing settings
/permission /tools /vim /config                         how zaino behaves
/rewind /journey /sessions /clear /compact /limit       the conversation and its tree
/agents /usage /help /quit
```

Write your own as `commands/<name>.md` in the config directory; `$ARGUMENTS`
stands in for whatever followed the name.

## Configuration

Two files with the same shape, project over user, and a flag beats both:

```
~/.config/zaino/config.json
<project>/.zaino/config.json
```

```json
{
  "provider": "anthropic",
  "model": "claude-opus-5",
  "permission": "accept-edits",
  "allow": ["bash:git status", "bash:go test"],
  "deny": ["read:.env"],
  "profiles": {
    "cheap": {"model": "claude-haiku-4-5", "effort": "low"},
    "deep":  {"model": "claude-opus-5", "effort": "max", "thinking": true}
  }
}
```

`system.md` beside it is the system prompt; `agents/*.md` describes subagents
`task` can run by name; `mcp.json` declares MCP servers. A `ZAINO.md` in the
repository is read every run and says where zaino is, not what it is for.

## Development

```bash
go build ./cmd/zaino       # build
go test ./...              # the test suite, including the same tool round-trip through every provider
go run ./cmd/zaino -log wire.jsonl   # record every request and response, credentials blanked
```

## License

[MIT](LICENSE).
