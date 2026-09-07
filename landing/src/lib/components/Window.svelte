<script>
  let { title = '', tilt = 0, children } = $props();
</script>

<div class="window lift" style:rotate={tilt ? `${tilt}deg` : null}>
  <p class="bar">
    <span class="dot"></span><span class="dot"></span><span class="dot"></span>
    <span class="title">{title}</span>
  </p>
  {@render children()}
</div>

<style>
  .window {
    background: var(--term); color: var(--term-fg);
    border: 3px solid var(--line); border-radius: 10px;
    box-shadow: 7px 7px 0 var(--shadow);
    font-family: var(--mono);
    overflow: hidden;
  }
  .window:hover { box-shadow: 9px 9px 0 var(--shadow); }

  .bar {
    display: flex; align-items: center; gap: 7px; margin: 0;
    padding: 10px 14px;
    border-bottom: 2px dashed color-mix(in oklab, var(--term-fg) 25%, transparent);
    font-size: 11px; letter-spacing: .12em; text-transform: uppercase; color: #857a67;
  }
  .dot { width: 10px; height: 10px; border-radius: 50%; background: #857a67; opacity: .6; }
  .title { margin-left: 8px; }
</style>
