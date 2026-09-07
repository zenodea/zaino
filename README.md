<p align="center">
  <img alt="zaino" src="assets/backpack.png" width="128">
</p>
<p align="center">
  <a href="https://go.dev"><img alt="Go" src="https://img.shields.io/badge/go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white" /></a>
  <a href="LICENSE"><img alt="MIT" src="https://img.shields.io/badge/license-MIT-c4522a?style=flat-square" /></a>
</p>

# Zaino

A coding agent in your terminal, packed into one Go binary. *Zaino* is
Italian for backpack. The provider clients, the turn loop, the tools and the
UI are all written in this repo. The three dependencies are for drawing.

* **Five providers** over their raw HTTP APIs: Anthropic, Gemini, OpenAI, Grok, OpenRouter
* **Eight tools for the code**: `read`, `write`, `edit`, `bash`, `grep`, `find`, `ls`, `fetch`
* **Four for the work around it**: `task` hands a job to a second agent, `job` reads or stops what `bash` left running, `memory` keeps notes across sessions, `todo` holds the plan
* **MCP servers** over stdio, and **hooks** you write yourself around every tool call
* **A full-screen UI** with a walkable transcript and vim editing, and a plain REPL for pipes
* **Sessions as a tree**: resume, `/rewind`, branch, see it all on a `/journey` map, and have the files put back to match

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/zenodea/Zaino/main/install.sh | sh
```

Or from a checkout: `./install.sh`, or `go install ./cmd/zaino`. The binary
lands in `~/.local/bin`, and `install.sh --uninstall` removes it.

## Running

Set a key for whichever provider you have and zaino picks it up:

```bash
export ANTHROPIC_API_KEY=...     # or GEMINI_API_KEY, OPENAI_API_KEY, XAI_API_KEY, OPENROUTER_API_KEY

zaino                            # full-screen UI, provider auto-detected
zaino -provider gemini -model gemini-2.5-flash
zaino -c                         # continue the last session here
```

Anything piped in gets the line-based REPL. `-p` asks once and exits, and
`-json` wraps the answer with the usage and the cost:

```bash
git diff | zaino -p "review this" -permission plan
zaino -p "what does install.sh do" -json | jq -r .answer
```

`zaino -h` lists the rest of the flags. Every one of them can also be set
once in the config. `-max-spend 5` stops the session once it has cost five
dollars, subagents and compaction included, and `/spend` shows or moves the
cap from inside.

## What's in the pack

| Path | What it holds |
|------|---------------|
| `internal/llm/` | The provider-neutral model: messages, blocks, streams |
| `internal/provider/` | One client per provider, each knowing only its own wire format |
| `internal/agent/` | The turn loop (stream, run the tools, feed back, repeat), compaction, subagents, the budget |
| `internal/tool/` | The twelve tools, and the rules that keep writes honest |
| `internal/permission/` | Who may do what, and when to ask |
| `internal/hook/` | Your scripts, run around the loop |
| `internal/mcp/` | MCP servers, spawned on stdio |
| `internal/frontend/` | The Bubble Tea UI and the REPL |
| `internal/store/` | Sessions and their file checkpoints, remembered settings, prompt recall, the wire log |
| `cmd/zaino/` | Flags, config and wiring |

Only `internal/provider/` knows a wire format. Adding a provider means
implementing one interface and nothing else.

## Tools

`read`, `write`, `edit`, `bash`, `grep`, `find`, `ls` and `fetch` touch the
repository. `write` and `edit` refuse a file that changed on disk since the
model last read it, and refuse anything under `.git`. `bash` with
`background: true` starts a server or a watcher and returns at once with a
job id; every job is stopped when zaino exits.

The other four are how the model manages its own work:

* `task` hands a job to a second agent with its own context, which reports back in a paragraph. It runs under the same permission gate, cannot ask you anything, and nests two deep at most. Name your own in `agents/*.md`.
* `job` reads what a background command has printed so far, stops it, or lists what is running. `/jobs` shows you the same.
* `memory` keeps one-line notes about the project: how the tests run, what you asked it to stop doing. They live in `~/.local/share/zaino/memory/<project>.md`, outside the repo, and are read in at the start of every session there. `/memory` shows them.
* `todo` holds the plan for a longer task, ticked off as it goes. Progress shows in the status line and `/todo` shows the list.

`-tools read,grep` hands out a subset, `-exclude-tools bash` withholds one,
`-no-tools` takes them all away. `/tools` shows what the model has and which
of it will ask first. MCP servers declared in `mcp.json` add theirs as
`server__tool`.

## Permission

A mode says what the model may do without asking. `-permission` or the config
sets it, and `⇧⇥` cycles it while a turn is running:

```
manual         asks before every write, command and fetch   (the default)
accept-edits   edits go through; commands and fetches still ask
plan           read only: the model writes a plan with todo and stops
bypass         nothing asks; deny rules are the one thing still honoured
```

When zaino asks you see the tool, the target and a preview: the diff for an
edit, the command line for `bash`. `y` allows once, `a` allows that tool and
target for the rest of the session, `n` refuses. A refusal is not an error.
The model is told and carries on with what it can do.

Anything you would always say yes or no to goes in the config as a rule:
`"allow": ["bash:git status"]`, `"deny": ["read:.env"]`. Project rules add to
your own, and a deny rule holds even in `bypass`.

One rule sits under all the modes: **paths outside the working directory are
refused, not asked about**, symlinks resolved first. `-allow-outside` is the
only thing that lifts it, and a subagent inherits its parent's gate, so it is
not a way around. This holds for the file tools. A shell command can still
read whatever the shell can.

## Hooks

Scripts of your own that run at points in the loop, from the same config:

```json
{
  "hooks": {
    "session-start": [{"run": "git fetch -q"}],
    "pre-tool":      [{"tool": "bash", "run": "sh .zaino/guard.sh"}],
    "post-tool":     [{"tool": "edit,write", "run": "gofmt -l . 2>&1"}],
    "turn-end":      [{"run": "notify-send zaino done"}]
  }
}
```

Each runs with `sh -c` in the project directory. The event's details arrive
on stdin as JSON: the tool, its input, and for `post-tool` its result.
`ZAINO_EVENT`, `ZAINO_TOOL` and `ZAINO_SESSION` are in the environment for
one-liners that would rather not parse. `tool` narrows a hook to a comma
list of tools; without it the hook runs for all of them.

Exit 0 lets the call through, and whatever a `post-tool` hook prints goes
back to the model on the end of the result, which is how a formatter or a
linter gets a say after every edit. Exit 2 blocks: a `pre-tool` hook's
stderr goes back to the model as the error instead of the call, and a
`post-tool` hook's marks the result as one. Any other exit is reported to
you and otherwise ignored. Thirty seconds is the limit unless `timeout_ms`
says otherwise. `/hooks` lists what is set up. A project's hooks run after
your own.

## Sessions, rewinding, and the files

A session is one append-only file of everything that happened. `/rewind`
takes the conversation back to an earlier prompt: the prompt returns to the
composer, everything after it leaves the context, and nothing is deleted.
The turns you leave stay in the file on a branch of their own. `/journey`
draws the whole file, every turn on every branch, and lets you jump to any
point on it. `-continue` picks up the newest session in a directory and
`-resume` takes any prefix of an id. `/clear` marks where the context starts
and deletes nothing either.

`write` and `edit` keep what a file held before and after, on the turn that
changed it. Moving to another point on the tree then asks whether to put the
files back the way they were there, however many jumps and branches away. A
file changed by something other than zaino since is left alone unless you say
to overwrite it. `bash` writes are not tracked and so not undone. Nothing
under `.git` is ever touched: a restore shows up in `git status` like any
other edit, and commits made in between stay. `/files` lists what the current
branch changed. `-checkpoints=false`, or `"checkpoints": false`, turns the
whole thing off.

## The UI

`⏎` sends, `⌥⏎` makes a newline, `↑`/`↓` recall earlier prompts. `⌃j`/`⌃k`
walk a bar through the transcript, and `⏎` on a tool call opens it up. A
running turn does not lock the composer: type, and what you said goes in
with the next tool results. `esc` stops a turn. `⌃c` twice quits.

Vim editing is on by default (`-vim=false` turns it off): the usual motions,
counts, `dd`/`cw`/`x`, visual mode, `u` and `p`. The composer starts in
insert, so nothing changes until you press `esc`.

`@path` attaches a file. Text goes in fenced, PNG/JPEG/GIF/WebP go in as
pictures.

## Commands

Typing `/` opens a fuzzy menu. The ones that only look, such as `/usage`,
`/tools`, `/files` and `/config`, work mid-turn. The rest wait.

```
/model /provider /effort /thinking /system /profile    the model-facing settings
/permission /tools /hooks /vim /config                  how zaino behaves
/rewind /journey /sessions /clear /compact /limit       the conversation and its tree
/files /todo /memory /jobs                              what the model is doing, and did
/spend /usage /agents /help /quit
```

Write your own as `commands/<name>.md` in the config directory. `$ARGUMENTS`
stands in for whatever followed the name.

## Configuration

Two files with the same shape. The project's wins over yours, and a flag
beats both:

```
~/.config/zaino/config.json
<project>/.zaino/config.json
```

```json
{
  "provider": "anthropic",
  "model": "claude-opus-5",
  "permission": "accept-edits",
  "max-spend": 5,
  "allow": ["bash:git status", "bash:go test"],
  "deny": ["read:.env"],
  "profiles": {
    "cheap": {"model": "claude-haiku-4-5", "effort": "low"},
    "deep":  {"model": "claude-opus-5", "effort": "max", "thinking": true}
  },
  "windows": {"my-local-model": 32000}
}
```

What you pick at the prompt with `/provider`, `/model` and `/effort` is
where the next zaino in that project starts from. It is kept in
`~/.local/state/zaino/last.json`, ranks above your own config.json and below
the project's, and never beats a flag. A project you have not picked in yet
starts from the most recent pick anywhere. `-no-config` ignores it along
with the rest.

The context window follows the model: 200k for Claude, 1M for Gemini, and so
on. `windows` covers a model zaino has not heard of, and `-context-window`
pins it outright.

`system.md` beside the config is the system prompt. `agents/*.md` describes
subagents `task` can run by name. `mcp.json` declares MCP servers. A
`ZAINO.md` in the repository is read every run and says where zaino is, not
what it is for. The branch, what is changed and the last few commits go in
beside it, unless `-git=false` or `"git": false`. The model's own notes from
`memory` go in too.

## Development

```bash
go build ./cmd/zaino       # build
go test ./...              # the test suite, including the same tool round-trip through every provider
go run ./cmd/zaino -log wire.jsonl   # record every request and response, credentials blanked
```

## License

[MIT](LICENSE).
