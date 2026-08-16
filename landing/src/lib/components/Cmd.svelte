<script>
  let { cmd, solo = false } = $props();

  let label = $state('copy');

  async function copy() {
    try {
      await navigator.clipboard.writeText(cmd);
      label = 'copied';
    } catch {
      label = 'select it';
    }
    setTimeout(() => (label = 'copy'), 1600);
  }
</script>

<div class="cmd" class:solo>
  <code><span class="p">$</span> {cmd}</code>
  <button
    class="copy"
    class:done={label === 'copied'}
    type="button"
    aria-label="Copy"
    onclick={copy}>{label}</button
  >
</div>

<style>
  .cmd {
    display: flex; align-items: center; gap: 10px;
    background: var(--term); border-radius: 6px;
    padding: 11px 11px 11px 14px; overflow: hidden;
  }
  .cmd code {
    background: none; border: none; padding: 0 0 2px;
    flex: 1; min-width: 0; display: block; overflow-x: auto;
    color: var(--term-fg); font-size: 12.5px; white-space: nowrap;
    scrollbar-width: thin; scrollbar-color: var(--faint) transparent;
  }
  .p { color: var(--mustard); user-select: none; margin-right: 7px; }

  .copy {
    flex: none; cursor: pointer;
    font-family: var(--mono); font-size: 10.5px; font-weight: 700;
    letter-spacing: .05em; text-transform: uppercase;
    background: var(--mustard); color: #23201a;
    border: 2px solid #000; border-radius: 5px; padding: 3px 9px;
    transition: background .2s ease, color .2s ease, translate .12s ease;
  }
  .copy:hover { background: #f5bb45; }
  .copy:active { translate: 1px 1px; }
  .copy.done { background: var(--forest); color: var(--canvas); animation: took .4s cubic-bezier(.3,1.5,.5,1); }
  @keyframes took { 40% { scale: 1.12; } }

  .cmd.solo {
    max-width: 660px; margin: 0 auto; text-align: left;
    border: 3px solid var(--ink); border-radius: 8px;
    box-shadow: 6px 6px 0 rgba(0,0,0,.45);
  }
</style>
