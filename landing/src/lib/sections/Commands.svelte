<script>
  import { reveal } from '$lib/reveal.js';
  import { commands, keyHints } from '$lib/content.js';
  import Note from '$lib/components/Note.svelte';
</script>

<section class="band drive" id="drive">
  <div class="wrap" data-stagger use:reveal>
    <p class="kick">Commands</p>
    <h2 class="big">Driving it</h2>
    <p class="sub">
      A line starting with <code>/</code> is for zaino, not the model. The slash
      opens a fuzzy menu; a command given no value opens a picker. Your own go
      in <code>commands/&lt;name&gt;.md</code>.
    </p>

    <div class="cmds">
      {#each commands as column}
        <dl>
          {#each column as [name, desc] (name)}
            <dt>{name}</dt>
            <dd>{desc}</dd>
          {/each}
        </dl>
      {/each}
    </div>

    <div class="keys">
      {#each keyHints as { keys, label } (label)}
        <span>
          {#each keys as key (key)}<kbd>{key}</kbd>{/each}
          {label}
        </span>
      {/each}
    </div>

    <div class="notes">
      <Note>
        {#snippet title()}The mouse stays the terminal's{/snippet}
        <p>
          Select and copy the way you do anywhere else. Scrolling is on the
          keyboard; <code>-mouse</code> gives the wheel to zaino at the cost of
          <kbd>⇧</kbd>-drag to select.
        </p>
      </Note>

      <Note>
        {#snippet title()}Walk the transcript{/snippet}
        <p>
          <kbd>⌃j</kbd> and <kbd>⌃k</kbd> move a bar through the transcript.
          <kbd>⏎</kbd> on a tool call opens what it was asked and what it
          returned. A running turn does not lock the composer: type, and it goes
          in with the next tool results.
        </p>
      </Note>

      <Note>
        {#snippet title()}Come back to it{/snippet}
        <p>
          <code>-continue</code> picks up the newest session here,
          <code>-resume</code> any prefix of an id, settings and all. A fresh
          session starts on whatever provider, model and effort you last picked
          in this project.
        </p>
      </Note>
    </div>
  </div>
</section>

<style>
  .cmds { display: grid; grid-template-columns: 1fr 1fr; gap: 0 46px; }
  @media (max-width: 720px) { .cmds { grid-template-columns: 1fr; } }
  .cmds dl { margin: 0; }
  .cmds dt {
    float: left; clear: left; width: 8.6em; padding: 10px 0;
    font-family: var(--mono); font-size: 13.5px; font-weight: 700; color: var(--rust);
  }
  .cmds dt, .cmds dd { transition: color .2s ease; }
  .cmds dl > dt:hover, .cmds dl > dt:hover + dd { color: var(--ink); }

  .cmds dd {
    margin: 0; padding: 10px 0 10px 8.6em;
    border-bottom: 2px dotted color-mix(in oklab, var(--ink) 28%, transparent);
    font-size: 15.5px; color: var(--ink-2);
  }

  .keys { display: flex; flex-wrap: wrap; gap: 12px 24px; margin: 38px 0 0; font-size: 14px; color: var(--ink-2); }
  .cmds dt { width: 9.4em; }
  .cmds dd { padding-left: 9.4em; }
  .keys kbd { margin-right: 4px; }
</style>
