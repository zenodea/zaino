<script>
  import { reveal } from '$lib/reveal.js';
  import { tools } from '$lib/content.js';
  import Note from '$lib/components/Note.svelte';
</script>

<section class="band reach" id="reach">
  <div class="wrap" data-stagger use:reveal>
    <p class="kick">Tools</p>
    <h2 class="big">Eight tools, and one<br />that hands the work away.</h2>
    <p class="sub">
      Withhold one with <code>-exclude-tools bash</code>, hand out a subset with
      <code>-tools read,grep</code>, or turn the lot off with
      <code>-no-tools</code>. <code>/tools</code> lists what the model has.
    </p>

    <ul class="chips" data-stagger use:reveal>
      {#each tools as tool (tool)}
        <li>{tool}</li>
      {/each}
      <li class="hand">task</li>
    </ul>

    <div class="duo">
      <Note>
        {#snippet title()}<code>task</code>: a second agent{/snippet}
        <p>
          It runs with its own conversation and hands back only what it
          concluded. A search that reads twenty files costs this conversation
          one paragraph instead of twenty file contents.
        </p>
        <p>
          The child inherits the parent's gate, so it is not a way around a
          refusal: same policy, same approver. It cannot ask you anything
          itself, so the prompt has to carry everything it needs. Nesting stops
          two deep.
        </p>
      </Note>

      <div class="margin-notes">
        <p>
          <b>MCP</b> servers plug in over stdio, declared in
          <code>mcp.json</code>. Their tools arrive named
          <code>server__tool</code> and ask for approval like anything else
          that leaves the process.
        </p>
        <p>
          <b>fetch</b> returns a page with the markup stripped. It leaves the
          process too, so <code>manual</code> and <code>accept-edits</code>
          both stop to ask first.
        </p>
      </div>
    </div>
  </div>
</section>

<style>
  .reach {
    background: var(--paper);
    border-top: 3px solid var(--ink);
    border-bottom: 3px solid var(--ink);
  }

  .chips {
    display: flex; flex-wrap: wrap; gap: 9px; margin: 0 0 44px; padding: 0;
    list-style: none; --step-ms: 40ms;
  }
  .chips li {
    font-family: var(--mono); font-size: 13.5px; font-weight: 500;
    padding: 6px 16px;
    background: var(--canvas); color: var(--ink);
    border: 2.5px solid var(--ink); border-radius: 999px;
    box-shadow: 3px 3px 0 var(--shadow);
    transition: translate .18s ease, box-shadow .18s ease, rotate .18s ease;
  }
  .chips li:hover { translate: -1px -2px; rotate: -1.5deg; box-shadow: 4px 5px 0 var(--shadow); }
  .chips li.hand { background: var(--mustard); font-weight: 700; }

  .duo {
    display: grid; grid-template-columns: 1.1fr 1fr;
    gap: 26px 34px; align-items: start; margin-top: 46px;
  }
  @media (max-width: 900px) { .duo { grid-template-columns: 1fr; } }

  .margin-notes p {
    margin: 0; padding: 12px 2px;
    font-size: 14.5px; color: var(--ink-2);
    border-bottom: 2px dotted color-mix(in oklab, var(--ink) 28%, transparent);
  }
  .margin-notes p:first-child { padding-top: 4px; }
  .margin-notes b { font-family: var(--mono); font-size: 13px; color: var(--rust); }
</style>
