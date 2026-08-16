<script>
  import { installs } from '$lib/content.js';
  import Cmd from './Cmd.svelte';

  let selected = $state(installs[0].id);
  let shown = $derived(installs.find((i) => i.id === selected));
</script>

<div class="tag lift">
  <span class="tag-hole" aria-hidden="true"></span>
  <div class="tag-body">
    <div class="tag-tabs" role="tablist" aria-label="How to install">
      {#each installs as { id, label } (id)}
        <button
          role="tab"
          aria-selected={id === selected}
          onclick={() => (selected = id)}>{label}</button
        >
      {/each}
    </div>
    {#key selected}
      <div class="panel"><Cmd cmd={shown.cmd} /></div>
    {/key}
  </div>
</div>

<style>
  .tag {
    position: relative; display: flex; align-items: stretch;
    max-width: 620px;
    background: var(--paper);
    border: 3px solid var(--ink);
    border-radius: 10px;
    box-shadow: 6px 6px 0 var(--shadow);
    transform: rotate(-0.5deg);
  }
  .tag:hover { box-shadow: 8px 8px 0 var(--shadow); }

  .tag-hole {
    flex: none; width: 34px;
    border-right: 2px dashed var(--ink);
    background: var(--sand);
    border-radius: 7px 0 0 7px;
    position: relative;
  }
  .tag-hole::after {
    content: ""; position: absolute; left: 11px; top: 50%; margin-top: -6px;
    width: 12px; height: 12px; border-radius: 50%;
    border: 2px solid var(--ink); background: var(--canvas);
  }
  .tag-body { flex: 1; min-width: 0; padding: 10px 12px 12px; }

  .tag-tabs { display: flex; gap: 6px; margin-bottom: 9px; }
  .tag-tabs button {
    cursor: pointer;
    font-family: var(--mono); font-size: 11.5px; font-weight: 500;
    background: transparent; color: var(--ink-2);
    border: 2px solid transparent; border-radius: 20px; padding: 2px 10px;
    transition: background .2s ease, color .2s ease, border-color .2s ease;
  }
  .tag-tabs button:hover { color: var(--ink); }
  .tag-tabs button[aria-selected="true"] {
    background: var(--rust); color: #fff8ee; border-color: var(--ink);
    font-weight: 700;
  }

  .panel { animation: swap .22s ease-out; }
  @keyframes swap { from { opacity: 0; translate: 0 -4px; } }
</style>
