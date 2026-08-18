<script>
  import { reveal } from '$lib/reveal.js';
</script>

<section class="band packing" id="pack">
  <div class="wrap" data-stagger use:reveal>
    <h2 class="big">What's in the pack</h2>
    <p class="sub">
      The clients, the loop, the composer and the transcript are written in
      this repo, not wrapped from an SDK.
    </p>

    <div class="slip lift">
      <p class="slip-top">Packing list</p>
      <ul data-stagger use:reveal>
        <li>Streaming, and the turn loop that drives it</li>
        <li>Eight tools: <code>read write edit bash grep find ls fetch</code></li>
        <li><code>task</code>, which hands work to a second agent</li>
        <li>MCP servers, spoken over stdio</li>
        <li>Compaction when the context window fills</li>
        <li>
          Anthropic, Gemini, OpenAI, Grok <span class="amp">&amp;</span>
          OpenRouter, hand-rolled
        </li>
        <li>Two frontends: full-screen TUI and a plain REPL</li>
        <li>A vim-style composer, and a walkable transcript</li>
        <li>Permissions, sessions, prompt recall</li>
        <li>
          <code>/rewind</code> and <code>/journey</code>: the session as a
          tree, every branch kept
        </li>
        <li>Images, attached with <code>@path</code> or read from disk</li>
        <li>Prompt caching on Anthropic, marked by hand</li>
        <li>Config files under <code>~/.config/zaino</code>, with profiles</li>
        <li>Markdown rendering, wire logging</li>
      </ul>
      <span class="stamp-all" aria-hidden="true">all packed</span>
    </div>
  </div>
</section>

<style>
  .packing { background: var(--paper); border-top: 3px solid var(--ink); border-bottom: 3px solid var(--ink); }

  .slip {
    position: relative;
    max-width: 880px;
    margin-inline: auto;
    background: var(--canvas);
    border: 3px solid var(--ink);
    border-radius: 12px;
    box-shadow: 6px 6px 0 var(--shadow);
    padding: 20px 30px 26px;
    transform: rotate(-.4deg);
  }
  .slip:hover { box-shadow: 9px 9px 0 var(--shadow); }
  .slip code { white-space: normal; }   /* the long tool list is allowed to wrap */

  .slip-top {
    text-align: center; margin: 0 0 6px;
    font-family: var(--mono); font-size: 10.5px; font-weight: 700;
    letter-spacing: .16em; text-transform: uppercase; color: var(--faint);
    border-bottom: 2px dashed var(--ink); padding-bottom: 10px;
  }

  .slip ul {
    list-style: none; margin: 0; padding: 0;
    columns: 2; column-gap: 48px;
    --step-ms: 45ms;
  }
  @media (max-width: 760px) { .slip ul { columns: 1; } }

  .slip li {
    break-inside: avoid;
    font-size: 15.5px; padding: 9px 0 9px 32px; position: relative;
    border-bottom: 2px dotted color-mix(in oklab, var(--ink) 28%, transparent);
  }
  .slip li::before {
    content: "✔";
    position: absolute; left: 0; top: 11px;
    display: inline-grid; place-items: center;
    width: 19px; height: 19px;
    background: var(--forest); color: var(--canvas);
    border: 2px solid var(--ink); border-radius: 5px;
    font-family: var(--sans); font-size: 10px;
  }
  .amp { font-family: var(--disp); font-style: italic; color: var(--rust); }

  .stamp-all {
    position: absolute; right: 26px; bottom: -14px;
    font-family: var(--mono); font-size: 11px; font-weight: 700;
    letter-spacing: .12em; text-transform: uppercase;
    background: var(--mustard); color: #23201a;
    border: 2px solid var(--ink); border-radius: 5px;
    padding: 3px 11px;
    transform: rotate(-3deg);
    box-shadow: 2px 2px 0 var(--shadow);
  }
</style>
