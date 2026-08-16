<script>
  import { reveal } from '$lib/reveal.js';
  import Note from '$lib/components/Note.svelte';
</script>

<section class="band compact" id="compact">
  <div class="wrap two" data-stagger use:reveal>
    <div>
      <p class="kick">Compaction</p>
      <h2 class="big">A long session<br />stops fitting.</h2>
      <p class="body-copy">
        When the last turn's own token count passes the window less a reserve,
        everything but the recent stretch is folded into one summary and the
        conversation carries on from there. <code>/compact</code> does it on
        demand, <code>-context-window</code> says how much the model can hold,
        and <code>-no-compact</code> turns it off.
      </p>
      <p class="body-copy dim">
        The trigger uses what the provider counted for the last turn rather than
        an estimate, so it is the real context size that decides. The estimate is
        only a stand-in before the first turn has come back.
      </p>
    </div>

    <div class="notes-stack">
      <Note>
        {#snippet title()}The cut never strands a tool result{/snippet}
        <p>
          A result whose call landed on the other side of the boundary is
          rejected by the provider, so the cut moves forward past it.
        </p>
      </Note>

      <Note>
        {#snippet title()}A summary is a boundary, like a clear{/snippet}
        <p>
          It is written as its own entry and the kept messages are written again
          after it, so reading the log back is a matter of starting at the last
          boundary and going forwards. A later <code>/clear</code> drops the
          summary with everything else.
        </p>
      </Note>
    </div>
  </div>
</section>

<style>
  .compact {
    background: var(--sand);
    border-top: 3px solid var(--ink);
    border-bottom: 3px solid var(--ink);
  }
</style>
