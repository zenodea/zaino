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

    <div class="notes">
      <Note>
        {#snippet title()}<code>task</code> — a second agent{/snippet}
        <p>
          It runs with its own conversation and hands back only what it
          concluded. A search that reads twenty files costs this conversation one
          paragraph instead of twenty file contents, which is what keeps a long
          session inside the window.
        </p>
        <p>
          The child inherits the parent's gate, so it is not a way around a
          refusal — same policy, same approver. It cannot ask you anything
          itself, so the prompt has to carry everything it needs. Nesting stops
          two deep.
        </p>
      </Note>

      <Note>
        {#snippet title()}MCP, over stdio{/snippet}
        <p>
          Servers are declared in <code>mcp.json</code>, spawned on stdio and
          asked what they can do. Their tools arrive named
          <code>server__tool</code>, so two servers offering <code>search</code>
          stay apart.
        </p>
        <p>
          A server that will not start is reported and skipped rather than taken
          as fatal. Nothing is known about what a server does, so its tools ask
          for approval like anything else that leaves the process.
        </p>
      </Note>

      <Note>
        {#snippet title()}<code>fetch</code> — a URL, as text{/snippet}
        <p>
          It gets a page and returns it with the markup stripped from the HTML.
          It is the tool that lets the model check what an API actually
          documents, rather than what it remembers about it.
        </p>
        <p>
          Because it leaves the process, it has a permission action of its own:
          <code>manual</code> and <code>accept-edits</code> both stop to ask
          before anything is fetched.
        </p>
      </Note>
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
</style>
