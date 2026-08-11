# Zaino

An agent harness in Go, with hand-rolled provider clients and a terminal UI.

## Layout

    internal/llm/                Provider-neutral model: messages, content
                                 blocks, streaming events, and the accumulator
    internal/agent/              The turn loop: stream → run tools → repeat

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
`-no-save`, `-log`.

Keys: `⏎` send, `⌃j` newline, `↑`/`↓` earlier prompts, `⌃l` clear context,
`⌃c` cancel the running turn (again to quit), `⌃d` quit, `PgUp`/`PgDn` scroll.

## Commands

A line beginning with `/` acts on the session instead of going to the model:

    /help                list the commands
    /clear               forget the conversation      (/new, /reset)
    /model [id]          show or change the model
    /provider [name]     show or switch provider (clears the context)
    /effort [level]      show or set output effort
    /thinking [on|off]   show or hide the model's reasoning
    /system [prompt|-]   show, set, or drop the system prompt
    /usage               token usage for this session
    /sessions            pick up an earlier conversation   (/resume)
    /quit                leave zaino                  (/exit, /q)

Typing `/` opens a fuzzy-matched panel above the input: `↑`/`↓` choose, `⇥`
completes, `⏎` runs the highlighted command, `esc` dismisses. A prompt that
merely starts with a slash — `/etc/hosts is wrong` — is still a prompt.

`-plain` gives a line-based REPL instead of the full-screen UI; it takes the
same commands. It is selected automatically when stdin is not a terminal, so
piping works:

    echo "say hi" | go run ./cmd/zaino -v

## Status

Streaming, the turn loop, tool dispatch, both providers, and the UI are
implemented and tested — 53 tests, including the same tool round-trip driven
through each provider to prove the loop is provider-agnostic.

**Not yet verified against a live API.** Every test runs against a fake server,
so the wire formats are informed guesses until a real request goes out.

No tools are registered yet, so in practice the loop terminates on `end_turn`.

Not built: tools, permissions, MCP, subagents, context compaction, persistence.

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
