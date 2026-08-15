# Zaino

An agent harness in Go, with hand-rolled provider clients and a terminal UI.

## Layout

    internal/llm/                Provider-neutral model: messages, content
                                 blocks, streaming events, and the accumulator
    internal/agent/              The turn loop: stream → run tools → repeat
    internal/tool/               read, write, edit, bash, grep, find, ls, fetch
    internal/permission/         Who is allowed to do what, and who to ask
    internal/mcp/                MCP servers, spoken over stdio

    internal/provider/           Name → provider, with credential auto-detect
    internal/provider/anthropic/ Claude provider (Messages API)
    internal/provider/gemini/    Gemini provider (Generative Language API)

    internal/frontend/tui/       Bubble Tea interface
    internal/frontend/repl/      Line-based REPL, for pipes and -plain

    internal/store/session/      Conversations on disk, and the projection
                                 that turns one back into messages to send
    internal/store/recall/       The prompts you have typed before
    internal/store/wirelog/      What went to the provider, for debugging

    internal/x/httpx/            Shared transport and retry
    internal/x/sse/              Shared server-sent-event framing
    internal/x/fsx/              Atomic writes and directory creation
    internal/x/paths/            Where zaino keeps its data

    cmd/zaino/                   Flags, wiring, and the choice of frontend

Only the packages under `internal/provider/` know a wire format. The agent
loop and both frontends speak `llm` types alone, so adding a provider means
implementing `llm.Provider` and nothing else.

## Running

    export ANTHROPIC_API_KEY=sk-ant-...     # console.anthropic.com
    export GEMINI_API_KEY=...               # aistudio.google.com/apikey

    go run ./cmd/zaino                      # auto-detects a provider
    go run ./cmd/zaino -provider gemini
    go run ./cmd/zaino -provider gemini -model gemini-2.5-flash

Flags: `-provider`, `-model`, `-max-tokens`, `-effort` (Anthropic only),
`-system`, `-thinking`, `-plain`, `-v`, `-continue`/`-c`, `-resume`/`-r`,
`-no-save`, `-log`, `-permission`, `-allow-outside`, `-tools`,
`-exclude-tools`, `-no-tools`, `-no-subagents`, `-mcp`, `-no-mcp`, `-vim`,
`-mouse`, `-animate`, `-context-window`, `-no-compact`.

Keys: `⏎` send, `⌥⏎` newline, `⌃j`/`⌃k` walk the chat, `↑`/`↓` earlier prompts,
`⇧⇥` cycle permission mode, `PgUp`/`PgDn` and `⌃u`/`⌃d` scroll.

`esc` stops a running turn, but only once vim has nothing else for it to do.
It leaves insert mode, then visual mode, then drops a half-typed operator —
and only in plain normal mode does the next press stop the turn. With vim off
it stops the turn straight away, and with nothing running it does nothing.

`⌃c` also stops a running turn. With nothing running it arms the quit and says so
in the footer; a second `⌃c` leaves, and any other key stands it down — so a
stray press cannot be completed half a conversation later. `/exit` also works.
Nothing else quits, and nothing else clears: use `/clear`.

`⌃j`/`⌃k` move a bar down and up the transcript one thing at a time, and the
bar marks the line an entry starts on rather than every line it wraps over.
With a tool call under it, `⏎` opens it up to show the arguments it was called
with and everything it returned; an error that runs to several lines is shown
by its first until `⏎` asks for the rest. Typing anything hands the keyboard
back to the composer, and `⌃j` off the bottom does the same.

The bar walks the lines between two entries rather than jumping the gap,
leaving a tail that thins and dims behind it. It covers a long distance in
about the same time as a short one, and the tail never borrows the bar's own
glyph — two full bars would read as two cursors. The transcript eases into
place under it rather than jumping. `-animate=false` turns all of it off.

The footer carries the permission mode at all times — `⏸ manual`,
`⏵⏵ accept edits`, `◇ plan`, `⏵⏵ bypass` — next to the editing mode. When the
line is too narrow for everything, the key hints shorten and the modes stay.

The mouse belongs to the terminal, so selecting and copying works as it does
anywhere else. Scrolling is on the keyboard instead: `⌃j`/`⌃k`, `PgUp`/`PgDn`,
and `⌃u`/`⌃f`/`⌃b`.

`-mouse` gives the wheel to zaino if you would rather scroll with it. That
grabs the mouse, so selecting then needs `⇧`-drag (or `⌥`-drag on macOS).

What the model writes is rendered as markdown — bold, italics, inline code,
headings, lists, quotes and fenced code blocks. Your own prompts are shown as
typed, so a literal `**` stays one.

## Vim

Modal editing is on by default; `-vim=false` or `/vim off` turns it off. The
composer starts in **insert**, so nothing is different until you press `esc`.

    motions      h j k l w b e 0 ^ $ gg G, with counts: 3w, 2j
    insert       i a I A o O
    edits        x X D C dd cc dw cw db d$ d0 de, with counts: 3dd
    visual       v select · V by line · d c y on the selection · esc cancel
    other        u undo · p P paste what you last cut

The composer draws itself in visual mode, since the text box underneath it
cannot show a selected range.

`⌃j`/`⌃k` walk the transcript from either mode.

`⏎` always sends, in either mode: this is a composer, and that is what it is
for. `⌃u`/`⌃f`/`⌃b` scroll the transcript, and so do `j`/`k` while the prompt
is a single line — which it usually is. Once the prompt has more than one line
they move the cursor instead.

The session picker takes `j`/`k`, `g`/`G`, `⌃d`/`⌃u`, `l` or `⏎` to resume, and
`h` or `q` to go back. It replaces the screen while it is open rather than
sitting above a composer you cannot type into, fills the height it is given,
and reads like the transcript: oldest at the top, newest at the bottom, and the
cursor already down there.

## Commands

A line beginning with `/` acts on the session instead of going to the model:

    /help                list the commands
    /clear               forget the conversation      (/new, /reset)
    /model [id]          show or change the model
    /provider [name]     show or switch provider (clears the context)
    /effort [level]      show or set output effort
    /thinking [on|off]   show or hide the model's reasoning
    /system [prompt|-]   show, set, or drop the system prompt
    /permission [mode]   show or set when tools stop to ask  (/perm, /mode)
    /tools               list the tools the model has
    /compact             fold the conversation so far into a summary
    /vim [on|off]        modal editing in the composer
    /usage               token usage for this session
    /sessions            pick up an earlier conversation   (/resume)
    /quit                leave zaino                  (/exit, /q)

Every command that takes a value and is given none asks on a screen of its own,
with what each option actually means under the list:

- `/permission` shows a grid of what each mode permits — read, write, run,
  fetch. It is asked of a real policy rather than written down, so it cannot
  claim something the gate would not do.
- `/vim` shows the key map for whichever mode is under the cursor.
- `/thinking` shows what the transcript looks like either way.
- `/model` and `/provider` show what carries over and what does not.
- `/effort` is one dimension, so it lies across the screen instead: the stops
  light up to where you are, warming as the scale climbs. `←`/`→` turn it up
  and down, and `↑`/`↓` work too.

`/clear` and `/compact` say what they are about to do before doing it, with the
cursor on the harmless answer. `/clear !` and `/compact !` skip the question.

The commands that answer rather than ask get a screen too. `/usage` draws the
context window as a bar and says when the next turn will compact; `/tools`
marks each tool with what it will do under the mode in force; `/help` lays out
the commands and the key map; `/system` shows the prompt as rendered markdown.
`j`/`k` scroll them, `esc` goes back.

An option with nothing else on its row says its name and its meaning together,
with a blank line between it and the next. The bar walks the lines in between
rather than jumping the gap, leaving the same fading tail it does in the
transcript.

Typing `/` opens a fuzzy-matched panel above the input: `↑`/`↓` choose, `⇥`
completes, `⏎` runs the highlighted command. The panel is a function of what is
in the box, so it closes when the line stops looking like a command rather than
on a key of its own. A prompt that merely starts with a slash —
`/etc/hosts is wrong` — is still a prompt, and the panel knows it.

`-plain` gives a line-based REPL instead of the full-screen UI; it takes the
same commands. It is selected automatically when stdin is not a terminal, so
piping works:

    echo "say hi" | go run ./cmd/zaino -v

## Tools

Eight of them: `read`, `write`, `edit`, `bash`, `grep`, `find`, `ls`, `fetch`,
plus `task` for handing work to a second agent. Pick a subset with
`-tools read,grep`, withhold one with `-exclude-tools bash`, or turn the lot
off with `-no-tools`. `/tools` lists what the model has.

`edit` takes an `edits` array to make several changes to one file in a single
call. They are applied in order, each seeing the result of the last, and if any
one of them fails to match then none of them are written — a batch that stopped
half way through would leave the file in a state nobody asked for.

`fetch` gets a URL and returns it as text, with the markup stripped from HTML.
It is the tool that lets the model check what an API actually documents rather
than what it remembers.

Two rules keep `edit` and `write` honest, and they are about correctness rather
than permission:

- **Read it first.** A file has to have been read this session before it can be
  edited or overwritten, so nothing is written blind.
- **Say what you expect to find.** `edit` replaces an exact stretch of text that
  must appear exactly once. A near miss is forgiven where the difference can
  never matter — typographic quotes and dashes, trailing whitespace — and the
  result says when that happened.

If a file changes on disk between an edit being worked out and being allowed,
the write is refused rather than clobbering the other writer.

## Subagents

`task` runs a second agent with its own conversation and hands back only what
it concluded. A search that reads twenty files costs this conversation one
paragraph instead of twenty file contents, which is what keeps a long session
inside the window.

The child inherits the parent's gate, so a subagent is not a way around a
refusal: it asks with the same policy and the same approver. It cannot ask you
anything itself, so the prompt has to carry everything it needs. Nesting stops
two deep, and `-no-subagents` withholds it.

## MCP

Servers are declared in `mcp.json` beside the sessions, or wherever `-mcp`
points:

    {
      "servers": {
        "docs": {"command": "npx", "args": ["-y", "@some/mcp-server"]},
        "db":   {"command": "./bin/db-mcp", "env": {"DSN": "..."}}
      }
    }

Each server is spawned on stdio, asked what it can do, and its tools appear
named `server__tool` so two servers offering `search` stay apart. A server that
will not start is reported and skipped rather than taken as fatal. Nothing is
known about what a server does, so its tools ask for approval like anything
else that leaves the process. `-no-mcp` skips the lot.

## Compaction

A long session stops fitting. When the last turn's own token count passes the
window less a reserve, everything but the recent stretch is folded into one
summary and the conversation carries on from there. `/compact` does it on
demand, `-context-window` says how much the model can hold, and `-no-compact`
turns it off.

Two details matter more than the summarising itself:

- **The cut never strands a tool result.** A result whose call landed on the
  other side of the boundary is rejected by the provider, so the cut moves
  forward past it.
- **A summary is a boundary in the session, like a clear.** It is written as
  its own entry, the kept messages are written again after it, and reading the
  log back is a matter of starting at the last boundary and going forwards. A
  later `/clear` drops the summary with everything else.

The trigger uses what the provider counted for the last turn, not an estimate,
so it is the real context size that decides. The estimate is only a stand-in
before the first turn has come back.

## Permission

What the model may do without asking is a mode, and `⇧⇥` cycles it:

    manual         ask before writing, running or fetching   (the default)
    accept-edits   edits go through, ask before running or fetching
    plan           read only — nothing is written or run, but pages can be read
    bypass         everything goes through unasked

Set it with `-permission accept-edits` or `/permission plan`. Reading is never
gated in any mode except by the boundary below.

When zaino asks, `y` allows once, `a` allows that tool and target for the rest
of the session, `n` refuses. A refusal is not an error: the model is told it was
refused and carries on. Answers are recorded in the session, so a transcript
says what the model was allowed to touch.

Underneath the modes is one rule they cannot lift: **paths outside the working
directory are refused, not asked about**. Symlinks are resolved before the check,
so a link pointing out of the tree is still out of the tree. `-allow-outside`
is the only thing that changes this, and `bypass` is the only mode that ignores it.

Without a terminal — piped input, `-v` into a file — there is nobody to ask, so
anything that would prompt is refused. Use `-permission accept-edits` or
`bypass` if a non-interactive run is meant to write.

## Status

Streaming, the turn loop, tools, permissions, both providers, and the UI are
implemented and tested — 310 tests, including the same tool round-trip driven
through each provider to prove the loop is provider-agnostic.

Not built: branching — every entry already carries an `id` and a `parent`, so
the tree is what you get the first time the leaf is allowed to move backwards.

## History

Three separate things, all under `~/.local/share/zaino` (or `$XDG_DATA_HOME`):

`↑` in the composer walks back through prompts you have sent, `↓` walks
forward and hands back the draft you were part-way through. They are kept in
`recall`, one per line. The `-plain` REPL reads whole lines from stdin, so it
has no cursor to move and no recall.

Conversations are saved as they happen, one JSONL file per run under
`sessions/<directory>/`, so `-continue` means the newest one started here.
Every line is one thing that happened — a message, a model change, a cleared
context — and what gets sent to the model is worked out from that record
rather than stored. So `/clear` deletes nothing: it marks where the context
starts, and the transcript before it stays readable. `-resume <id>` takes any
prefix of an id, `/sessions` picks one from a list, and `-no-save` records
nothing.

Resuming restores the model, system prompt, effort and thinking as they were,
unless you pass the flag for one. A session that used another provider comes
back on that provider where it can; where it cannot, its reasoning blocks are
dropped, because they carry a signature only their own provider reads back.

`-log wire.jsonl` records every request and response, including streamed
bodies as they arrive, with the credential headers blanked out.
