<script>
  let { kind = 'stitch' } = $props();
</script>

<div class="seam {kind}" aria-hidden="true">
  <div class="line"></div>
  {#if kind === 'zip'}<div class="line second"></div>{/if}
  <span class="mark"></span>
</div>

<style>
  .seam { position: relative; height: 0; z-index: 1; }
  .line {
    position: absolute; left: 0; right: 0; top: -1.5px;
    border-top: 3px solid var(--line);
  }
  .mark {
    position: absolute; left: 50%; top: 0; translate: -50% -50%;
    display: block; background: var(--mustard);
    border: 2px solid var(--line); box-shadow: 2px 2px 0 var(--shadow);
  }

  .stitch .mark { width: 11px; height: 11px; rotate: 45deg; }

  .zip .line.second { top: 4px; border-top: 2px dotted var(--line); opacity: .5; }
  .zip .mark { width: 20px; height: 12px; border-radius: 3px; }

  .tape .mark { width: 44px; height: 8px; border-radius: 2px; }

  .buckle .mark { width: 24px; height: 16px; border-radius: 3px; }
  .buckle .mark::after {
    content: ""; position: absolute; left: 5px; right: 5px; top: 5px; height: 2px;
    background: var(--ink); border-radius: 1px;
  }
</style>
