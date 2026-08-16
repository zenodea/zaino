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
      A line starting with <code>/</code> acts on the session instead of going to
      the model. A prompt that merely starts with a slash —
      <code>/etc/hosts is wrong</code> — is still a prompt, and the panel knows
      it.
    </p>
    <p class="sub">
      A command that takes a value and is given none asks instead of explaining:
      it opens a list with what is currently in effect marked. Passing the value
      outright — <code>/effort low</code> — skips it.
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
        {#snippet title()}Modal editing, on by default{/snippet}
        <p>
          The composer starts in <b>insert</b>, so nothing is different until you
          press <kbd>esc</kbd>. Then motions, operators with counts, and a visual
          mode the composer draws itself, since the text box underneath can't
          show a selected range. <code>-vim=false</code> turns it off.
        </p>
      </Note>

      <Note>
        {#snippet title()}Walk the transcript{/snippet}
        <p>
          <kbd>⌃j</kbd> and <kbd>⌃k</kbd> move a bar through the transcript one
          entry at a time. With a tool call under the bar, <kbd>⏎</kbd> opens the
          arguments it was called with and everything it returned. Typing hands
          the keyboard back.
        </p>
      </Note>

      <Note>
        {#snippet title()}The mouse is left to the terminal{/snippet}
        <p>
          Select and copy the way you do anywhere else; scrolling is on the
          keyboard instead. <code>-mouse</code> gives the wheel to zaino, at the
          cost of needing <kbd>⇧</kbd>-drag to select.
        </p>
      </Note>

      <Note>
        {#snippet title()}<code>/clear</code> deletes nothing{/snippet}
        <p>
          A session is one append-only file of things that happened. What gets
          sent is worked out from that record, so clearing just marks where the
          context starts — the transcript before it stays readable.
        </p>
      </Note>

      <Note>
        {#snippet title()}Come back to it{/snippet}
        <p>
          <code>-continue</code> is the newest run from this directory;
          <code>-resume</code> takes any prefix of an id. The model, prompt,
          effort and thinking come back as you left them.
        </p>
      </Note>

      <Note>
        {#snippet title()}Stopping, and leaving{/snippet}
        <p>
          <kbd>esc</kbd> stops a running turn — but only once vim has nothing
          else for it to do. <kbd>⌃c</kbd> stops it too; with nothing running it
          arms the quit and says so in the footer, and any other key stands it
          down.
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
  .keys kbd { margin-right: 4px; }
</style>
