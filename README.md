<div align="center">

# Zaino

A coding agent that lives in your terminal: its own provider clients,
its own turn loop, its own UI, all in Go.

*Zaino* is Italian for backpack. Everything the agent needs, packed in
one binary — no SDKs, and three dependencies, all of them for drawing.

</div>

## The idea

You type, the model answers, and when it wants to read a file or run a
command it asks a tool — and sometimes you — first. That loop is the whole
program: stream a response, run the tools it called, feed the results back,
repeat.

Zaino speaks to Claude and Gemini over their raw HTTP APIs with clients
written here, not imported. Only the packages under `internal/provider/`
know a wire format; the loop and both frontends speak one neutral set of
types, so adding a provider means implementing one interface and nothing
else.

## Running

    export ANTHROPIC_API_KEY=sk-ant-...     # console.anthropic.com
    export GEMINI_API_KEY=...               # aistudio.google.com/apikey

    go run ./cmd/zaino                      # auto-detects a provider
    go run ./cmd/zaino -provider gemini -model gemini-2.5-flash

Flags: `-provider`, `-model`, `-max-tokens`, `-effort` (Anthropic only),
`-system`, `-thinking`, `-plain`, `-v`, `-continue`/`-c`, `-resume`/`-r`,
`-no-save`, `-log`, `-permission`, `-allow-outside`, `-tools`,
`-exclude-tools`, `-no-tools`, `-no-subagents`, `-mcp`, `-no-mcp`, `-vim`,
`-mouse`, `-animate`, `-context-window`, `-no-compact`, `-max-context`,
`-profile`, `-no-config`.

`-system` takes the prompt itself, or `@` and a file holding one:
`-system @prompts/terse.md`.

## The UI

The full-screen UI is where you will spend your time. `⏎` sends, `⌥⏎` makes
a newline, `↑`/`↓` walk back through prompts you have sent before, and `⇧⇥`
cycles the permission mode. What the model writes renders as markdown; what
you write stays exactly as you typed it.

`⌃j`/`⌃k` move a bar up and down the transcript, one thing at a time. Park
it on a tool call and `⏎` opens the call up — arguments, results, the whole
error if there was one. The bar walks the lines between entries rather than
jumping, leaving a tail that thins and dims behind it. `-animate=false`
turns the theatrics off.

`esc` and `⌃c` stop a running turn. With nothing running, `⌃c` arms the
quit and says so in the footer; a second press leaves, any other key stands
it down — a stray `⌃c` cannot finish itself half a conversation later.
Nothing else quits, and nothing else clears: that is what `/clear` is for.

The mouse belongs to the terminal, so selecting and copying work as they do
anywhere else. `-mouse` hands the wheel to zaino instead, if you would
rather scroll with it.

For pipes and scripts there is `-plain`, a line-based REPL that takes the
same commands. It is picked automatically when stdin is not a terminal:

    echo "say hi" | go run ./cmd/zaino -v

## Tools

Eight tools: `read`, `write`, `edit`, `bash`, `grep`, `find`, `ls`,
`fetch` — plus `task`, for handing work to a second agent. Pick a subset
with `-tools read,grep`, withhold one with `-exclude-tools bash`, or turn
the lot off with `-no-tools`.

Two rules keep the writing tools honest, and they are about correctness
rather than permission:

- **Read it first.** A file must have been read this session before it can
  be edited or overwritten. Nothing is written blind.
- **Say what you expect to find.** `edit` replaces an exact stretch of text
  that must appear exactly once, and a batch of edits to one file lands
  entirely or not at all.

If a file changes on disk between an edit being worked out and being
allowed, the write is refused rather than clobbering the other writer.

`fetch` gets a URL back as text with the markup stripped — the tool that
lets the model check what an API actually documents rather than what it
remembers.

## Permission

What the model may do without asking is a mode, and `⇧⇥` cycles it:

    manual         ask before writing, running or fetching   (the default)
    accept-edits   edits go through, ask before running or fetching
    plan           read only — nothing is written or run
    bypass         everything goes through unasked

When zaino asks, `y` allows once, `a` allows that tool and target for the
session, `n` refuses. A refusal is not an error: the model is told, and
carries on. Standing answers go in the config — `"allow": ["bash:git
status"]` covers commands that start that way, and `deny` holds even where
zaino would not otherwise ask.

Underneath the modes sits one rule they cannot lift: **paths outside the
working directory are refused, not asked about**. Symlinks are resolved
first, so a link pointing out of the tree is still out of the tree.
`-allow-outside` is the only thing that changes this.

## Subagents

`task` runs a second agent with its own conversation and hands back only
what it concluded. A search that reads twenty files costs this conversation
one paragraph instead of twenty file contents — which is what keeps a long
session inside the window.

The child inherits the parent's gate, so a subagent is not a way around a
refusal. Its work stays out of the main transcript: the call is a live card
that says what the child is doing, and `/agents` lists every child this
session has spawned. `⏎` walks into one and shows its whole conversation,
live while it runs; `x` stops it without touching the rest of the turn.
Children are recorded with the session, so a resumed conversation still has
them to walk into. Nesting stops two deep, and `-no-subagents` withholds
the tool entirely.

Describe your own agents in `agents/*.md` — what each is for, which tools
it may use, which model it runs on — and `task` offers them by name. See
Configuration, below.

## Pictures

An `@` and a path attaches an image to what you are typing, which is what a
file dropped onto the terminal leaves behind:

    why is the spacing wrong in @shot.png

PNG, JPEG, GIF and WebP, up to five megabytes. A prompt that merely
mentions `shot.png` is only talking about it; the `@` is what sends it. A
mention that will not load stops the turn rather than quietly going
without. `read` hands back a picture too, so the model can look at anything
in the workspace on its own.

## Sessions

Conversations are saved as they happen, one JSONL file per run. Every line
is one thing that happened — a message, a model change, a cleared context —
and what gets sent to the model is worked out from that record rather than
stored. So `/clear` deletes nothing: it marks where the context starts, and
the transcript before it stays readable.

`-continue` picks up the newest session started here, `-resume <id>` takes
any prefix of an id, and `/sessions` picks from a list. Resuming restores
the model, system prompt, effort and thinking as they were.

`/rewind` takes the conversation up again from an earlier turn: pick one of
your own prompts and it comes back to the composer to be changed and asked
again, with everything after it out of the context. Nothing is deleted —
the old entries stay in the file on a branch of their own, so the session
file has always been a tree, and this is the first thing that makes one.

`/journey` is that tree seen whole, drawn as a road map: the way that leads
to where you are is paved heavy, the branches `/rewind` left behind trail
off dashed, and what happened along the way — a compaction, a clear, a
change of model — stands on the roadside where it happened. `⏎` on any stop
travels there; `b` branches from it instead.

## Context

A long session stops fitting. When the context passes the window less a
reserve, everything but the recent stretch is folded into one summary and
the conversation carries on from there. `/compact` does it on demand,
`-no-compact` turns it off, and the trigger uses what the provider actually
counted, not an estimate.

Anthropic's prompt caching is marked so every turn after the first reads
the prefix back at a tenth of the price; Gemini caches what it recognises
on its own. And if you want a hard stop rather than a longer walk,
`/limit 200k` gives the session a ceiling: the turn that would pass it is
held before it is sent, and `⏎` sends it anyway — consent covers that turn,
not the session.

## Configuration

Two config files with the same shape, and a flag beats both:

    ~/.config/zaino/config.json      yours
    <project>/.zaino/config.json     the project's

The keys are the flag names, so anything you can pass you can also settle
on once:

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

The project's answer wins where the two disagree, except that `allow` and
`deny` add up. A key that is not a setting is an error naming the file,
because a typo that goes quiet is worse than one that stops you.

**Profiles** are named bundles of the model-facing settings — the cheap
one, the thorough one. `-profile deep` picks one and `/profile` switches
mid-session.

**The system prompt** is `~/.config/zaino/system.md`, or the `system` key.
**ZAINO.md** is not a setting: it is read from the repository root down to
the working directory and appended fresh every run — it says where zaino is
running, not what it is for.

**Commands** are prompts you have written down. `commands/review.md`
becomes `/review`, and what it says is sent as if you had typed it, with
`$ARGUMENTS` standing in for whatever followed the name. `/bro` ships as
one of these — it asks for the last answer again, simply — and your own
`commands/bro.md` replaces it outright.

**MCP servers** are declared in `mcp.json`, spawned on stdio, and their
tools appear named `server__tool`. Nothing is known about what a server
does, so its tools ask for approval like anything else that leaves the
process. A server that will not start is reported and skipped rather than
taken as fatal.

## Commands

A line beginning with `/` acts on the session instead of going to the
model. Typing `/` opens a fuzzy-matched panel; a prompt that merely starts
with a slash — `/etc/hosts is wrong` — is still a prompt, and the panel
knows it.

    /help                list the commands
    /clear               forget the conversation      (/new, /reset)
    /model [id]          show or change the model
    /provider [name]     show or switch provider (clears the context)
    /effort [level]      show or set output effort
    /thinking [on|off]   show or hide the model's reasoning
    /system [prompt|-]   show, set, or drop the system prompt
    /bro                 say the last answer again, simply
    /profile [name]      switch to a named bundle of settings
    /config              what the config files came to, and where they are
    /permission [mode]   show or set when tools stop to ask  (/perm, /mode)
    /tools               list the tools the model has
    /rewind [prompt]     take the conversation up again from an earlier turn
    /journey             the tree of turns, every branch included   (/tree)
    /agents              the child agents spawned this session      (/tasks)
    /compact             fold the conversation so far into a summary
    /limit [tokens|off]  stop the session when the context passes a ceiling
    /vim [on|off]        modal editing in the composer
    /usage               token usage for this session
    /sessions            pick up an earlier conversation   (/resume)
    /quit                leave zaino                  (/exit, /q)

Every command that takes a value and is given none asks on a screen of its
own, with what each option actually means under the list — `/permission`
shows a grid of what each mode permits, asked of the real policy rather
than written down; `/effort` lies across the screen with its stops warming
as the scale climbs. `/clear` and `/compact` say what they are about to do
first, and `/clear !` skips the question.

## Vim

Modal editing is on by default; `-vim=false` or `/vim off` turns it off.
The composer starts in insert, so nothing is different until you press
`esc`.

    motions      h j k l w b e 0 ^ $ gg G, with counts: 3w, 2j
    insert       i a I A o O
    edits        x X D C dd cc dw cw db d$ d0 de, with counts: 3dd
    visual       v select · V by line · d c y on the selection
    other        u undo · p P paste what you last cut

`⏎` always sends, in either mode: this is a composer, and that is what it
is for.

## Layout

    internal/llm/          the provider-neutral model: messages, blocks, streams
    internal/agent/        the turn loop: stream → run tools → repeat
    internal/tool/         read, write, edit, bash, grep, find, ls, fetch
    internal/permission/   who may do what, and who to ask
    internal/mcp/          MCP servers, spoken over stdio
    internal/provider/     anthropic and gemini, each knowing its own wire
    internal/frontend/     the Bubble Tea UI, and the REPL for pipes
    internal/store/        sessions on disk, prompt recall, the wire log
    internal/x/            transport, SSE framing, atomic writes, paths
    cmd/zaino/             flags, wiring, and the choice of frontend

`-log wire.jsonl` records every request and response as they happen, with
the credential headers blanked out — what actually went over the wire, for
when you want to see for yourself.

## Status

Streaming, the turn loop, tools, permissions, both providers and the UI are
implemented and tested — 722 tests, including the same tool round-trip
driven through each provider to prove the loop is provider-agnostic. Not
built yet: pruning. A branch can always be gone back to, but never deleted;
the session file only grows.

## Built with

Go, and the Charm trio — Bubble Tea, Bubbles, Lip Gloss — for the drawing.
Everything else, from the HTTP retries to the SSE framing to the provider
clients, is written here. That is rather the point.
