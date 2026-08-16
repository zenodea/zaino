<script>
  let { title, children } = $props();
</script>

<div class="note lift">
  <h3>{@render title()}</h3>
  {@render children()}
</div>

<style>
  .note {
    background: var(--paper); border: 3px solid var(--ink); border-radius: 12px;
    box-shadow: 5px 5px 0 var(--shadow); padding: 22px 22px 18px;
  }
  .note:hover { box-shadow: 7px 7px 0 var(--shadow); }
  .note:nth-child(2) { transform: rotate(-.5deg); }
  .note h3 { font-size: 21px; margin-bottom: 8px; }
  .note h3 :global(code) { font-size: .78em; }
  .note :global(p) { font-size: 14.5px; color: var(--ink-2); margin: 0; }
  .note :global(p + p) { margin-top: 11px; }
</style>
