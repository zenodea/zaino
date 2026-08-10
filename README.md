# Zaino

An agent harness in Go, with hand-rolled provider clients and a terminal UI.

## Layout

    internal/llm/         Provider-neutral model: messages, content blocks,
                          streaming events, and the event accumulator
    internal/anthropic/   Claude provider (Messages API)
    internal/gemini/      Gemini provider (Generative Language API)
    internal/provider/    Name → provider, with credential auto-detect
    internal/agent/       The turn loop: stream → run tools → repeat
    internal/ui/          Bubble Tea interface
    internal/httpx/       Shared transport and retry
    internal/sse/         Shared server-sent-event framing
    cmd/zaino/            Entry point; full-screen UI plus a plain REPL

Only `internal/anthropic` and `internal/gemini` know a wire format. The agent
loop and the UI speak `llm` types alone, so adding a provider means
implementing `llm.Provider` and nothing else.

## Running

    export ANTHROPIC_API_KEY=sk-ant-...     # console.anthropic.com
    export GEMINI_API_KEY=...               # aistudio.google.com/apikey

    go run ./cmd/zaino                      # auto-detects a provider
    go run ./cmd/zaino -provider gemini
    go run ./cmd/zaino -provider gemini -model gemini-2.5-flash

Flags: `-provider`, `-model`, `-max-tokens`, `-effort` (Anthropic only),
`-system`, `-thinking`, `-plain`, `-v`.

Keys: `⏎` send, `⌃j` newline, `⌃l` clear context, `⌃c` cancel the running turn
(again to quit), `⌃d` quit, `PgUp`/`PgDn` scroll.

`-plain` gives a line-based REPL instead of the full-screen UI; it is selected
automatically when stdin is not a terminal, so piping works:

    echo "say hi" | go run ./cmd/zaino -v

## Status

Streaming, the turn loop, tool dispatch, both providers, and the UI are
implemented and tested — 36 tests, including the same tool round-trip driven
through each provider to prove the loop is provider-agnostic.

**Not yet verified against a live API.** Every test runs against a fake server,
so the wire formats are informed guesses until a real request goes out.

No tools are registered yet, so in practice the loop terminates on `end_turn`.

Not built: tools, permissions, MCP, subagents, context compaction, persistence.
