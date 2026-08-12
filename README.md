random
# Zaino

An agent harness in Go, with hand-rolled provider clients and a terminal UI.

## Layout

    internal/llm/                Provider-neutral model: messages, content
                                 blocks, streaming events, and the accumulator
    internal/agent/              The turn loop: stream → run tools → repeat
    internal/tool/               read, write, edit, bash, grep, find, ls
    internal/permission/         Who is allowed to do what, and who to ask

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
`-exclude-tools`, `-no-tools`, `-vim`, `-mouse`, `-animate`.

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

`⌃j`/`⌃k` move a bar down and up the transcript one thing at a time. With a
tool call under the bar, `⏎` opens it up to show the arguments it was called
with and everything it returned; `⏎` again closes it. Typing anything hands the
keyboard back to the composer, and `⌃j` off the bottom does the same.

The bar leaves a tail behind it — the mark on the entry you came from thins
and dims over the next few frames, so a run of `⌃k` draws a comet down the
gutter. It tapers by distance rather than by clock, so walking quickly still
reads as a tail instead of a row of identical marks. The transcript eases
into place under it rather than jumping. `-animate=false` turns all of it off.

The footer carries the permission mode at all times — `⏸ manual`,
`⏵⏵ accept edits`, `◇ plan`, `⏵⏵ bypass` — next to the editing mode. When the
line is too narrow for everything, the key hints shorten and the modes stay.

The wheel scrolls the transcript, and the session picker. That needs zaino to
grab the mouse, which takes text selection away from the terminal — hold
`⇧` (or `⌥` on macOS) while dragging to select anyway, as in any other
full-screen terminal program.

If you would rather have plain selection back, `-mouse=false` hands the mouse
to the terminal. Be aware of what that costs: in the alternate screen most
terminals turn the wheel into `↑`/`↓`, which lands on prompt recall rather than
scrolling. `PgUp`/`PgDn` still move the transcript either way.

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
    /vim [on|off]        modal editing in the composer
    /usage               token usage for this session
    /sessions            pick up an earlier conversation   (/resume)
    /quit                leave zaino                  (/exit, /q)

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

Seven of them: `read`, `write`, `edit`, `bash`, `grep`, `find`, `ls`. Pick a
subset with `-tools read,grep`, withhold one with `-exclude-tools bash`, or
turn the lot off with `-no-tools`. `/tools` lists what the model has.

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

## Permission

What the model may do without asking is a mode, and `⇧⇥` cycles it:

    manual         ask before writing or running anything    (the default)
    accept-edits   edits go through, ask before running anything
    plan           read only, nothing is written or run
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
implemented and tested — 220 tests, including the same tool round-trip driven
through each provider to prove the loop is provider-agnostic.

Not built: MCP, subagents, context compaction.

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
