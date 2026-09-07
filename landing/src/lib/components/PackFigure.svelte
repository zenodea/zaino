<script>
  import { packParts } from '$lib/content.js';
  import { reveal } from '$lib/reveal.js';
  import Pack3D from './Pack3D.svelte';

  let { delay = 0 } = $props();

  const spots = {
    bedroll: [0.49, 0.2, 'left'],
    lid: [0.6, 0.37, 'right'],
    main: [0.45, 0.53, 'left'],
    buckle: [0.575, 0.53, 'right'],
    side: [0.72, 0.64, 'right'],
    pocket: [0.47, 0.74, 'left'],
    base: [0.52, 0.86, 'right']
  };
  const rows = { left: [120, 290, 455], right: [190, 286, 376, 496] };
  const box = { x: 150, y: 24, w: 460 };
  const callouts = packParts.map((p, i) => {
    const [fx, fy, side] = spots[p.id];
    const dot = [box.x + fx * box.w, box.y + fy * box.w];
    const y = rows[side][side === 'left' ? [0, 1, 2][['bedroll', 'main', 'pocket'].indexOf(p.id)] : ['lid', 'buckle', 'side', 'base'].indexOf(p.id)];
    const edge = side === 'left' ? 146 : 588;
    const elbow = side === 'left' ? dot[0] - 40 : dot[0] + 40;
    return { ...p, side, dot, y, step: i, d: `M ${edge} ${y} H ${elbow} L ${dot[0]} ${dot[1]}` };
  });
</script>

<div class="fig-wrap" data-reveal use:reveal={{ delay }}>
  <figure class="fig">
    <div class="stage">
      <Pack3D size={460} pixels={128} spin={false} slide />
      <svg class="callouts" viewBox="0 0 760 530" aria-hidden="true">
        {#each callouts as c (c.id)}
          <g class="callout" style:--step={c.step}>
            <path class="lead" pathLength="1" d={c.d} />
            <circle class="dot" cx={c.dot[0]} cy={c.dot[1]} r="5" />
            <text class="part" x={c.side === 'left' ? 138 : 596} y={c.y - 12} text-anchor={c.side === 'left' ? 'end' : 'start'}>{c.part.toUpperCase()}</text>
            <text class="feat" x={c.side === 'left' ? 138 : 596} y={c.y + 12} text-anchor={c.side === 'left' ? 'end' : 'start'}>{c.feat}</text>
          </g>
        {/each}
      </svg>
    </div>
    <figcaption>fig. 1: the pack, from the outside</figcaption>
  </figure>

  <ul class="pack-key">
    {#each packParts as { part, feat } (part)}
      <li><b>{part}</b> {feat}</li>
    {/each}
  </ul>
</div>

<style>
  .fig-wrap, .fig { margin: 0; }
  :root[data-js] .fig-wrap:not([data-revealed]) { translate: 0 0; }
  .stage { position: relative; width: 100%; aspect-ratio: 760 / 530; }
  .stage :global(.pack3d-wrap) {
    position: absolute; left: calc(150 / 760 * 100%); top: calc(24 / 530 * 100%);
    width: calc(460 / 760 * 100%);
  }
  .stage :global(.pack3d) { width: 100% !important; height: auto !important; }
  .callouts { position: absolute; inset: 0; width: 100%; height: 100%; overflow: visible; }

  .fig figcaption {
    text-align: center; margin-top: 6px;
    font-family: var(--mono); font-size: 11.5px; letter-spacing: .1em;
    text-transform: uppercase; color: var(--faint);
  }

  .lead { fill: none; stroke: var(--line); stroke-width: 2.5; opacity: .45; stroke-dasharray: 1; }
  .dot  { fill: var(--ink); transform-box: fill-box; transform-origin: center; }
  .part {
    font-family: var(--mono); font-size: 14.5px; font-weight: 700;
    letter-spacing: .08em; fill: var(--faint);
  }
  .feat {
    font-family: var(--disp); font-weight: 700; font-size: 22px;
    font-style: italic; fill: var(--ink);
  }

  :root[data-js] .fig-wrap:not([data-revealed]) .lead { stroke-dashoffset: 1; }
  :root[data-js] .fig-wrap:not([data-revealed]) .dot,
  :root[data-js] .fig-wrap:not([data-revealed]) .part,
  :root[data-js] .fig-wrap:not([data-revealed]) .feat { opacity: 0; }

  .fig-wrap:global([data-revealed]) .lead {
    animation: draw .5s ease-out backwards;
    animation-delay: calc(1.2s + var(--step) * .06s);
  }
  .fig-wrap:global([data-revealed]) .dot {
    animation: land .35s cubic-bezier(.3,1.6,.5,1) backwards;
    animation-delay: calc(1.15s + var(--step) * .06s);
  }
  .fig-wrap:global([data-revealed]) .part,
  .fig-wrap:global([data-revealed]) .feat {
    animation: name .45s ease-out backwards;
    animation-delay: calc(1.2s + var(--step) * .06s);
  }
  @keyframes draw { from { stroke-dashoffset: 1; } }
  @keyframes land { from { opacity: 0; scale: 0; } }
  @keyframes name { from { opacity: 0; translate: 0 5px; } }

  .pack-key { display: none; }

  @media (max-width: 700px) {
    .stage { aspect-ratio: 1; }
    .stage :global(.pack3d-wrap) { left: 0; top: 0; width: 100%; }
    .callouts { display: none; }
    .pack-key {
      display: grid; gap: 0; margin: 18px 0 0; padding: 0; list-style: none;
      border-top: 2px dotted color-mix(in oklab, var(--ink) 30%, transparent);
    }
    .pack-key li {
      display: flex; flex-wrap: wrap; gap: 4px 10px;
      padding: 9px 2px; font-size: 15px; color: var(--ink-2);
      border-bottom: 2px dotted color-mix(in oklab, var(--ink) 30%, transparent);
    }
    .pack-key b {
      font-family: var(--mono); font-size: 11.5px; font-weight: 700;
      letter-spacing: .08em; text-transform: uppercase; color: var(--faint);
      flex: none; width: 11em; padding-top: 3px;
    }
  }
</style>
