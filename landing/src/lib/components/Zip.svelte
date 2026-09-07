<script>
  const W = 30, CX = 15, EDGE = 2, SPREAD = 7, PITCH = 6, WEDGE = 54;
  const SLIDER = 38, HINGE = 4, TAB = 26;

  let scrollY = $state(0);
  let innerHeight = $state(0);
  let docHeight = $state(0);
  let top = $state(0);
  let bottom = $state(0);
  let H = $state(0);
  let dragging = $state(false);
  let angle = $state(0);

  const range = $derived(docHeight - innerHeight);
  const travel = $derived(H - SLIDER - 10);
  const sliderTop = $derived(range > 0 ? 4 + travel * Math.min(1, Math.max(0, scrollY / range)) : 4);

  const spread = (y) => {
    const d = sliderTop - y;
    if (d <= 0) return 0;
    if (d >= WEDGE) return 1;
    const t = d / WEDGE;
    return t * t * (3 - 2 * t);
  };
  const left = (y) => CX - spread(y) * SPREAD;
  const right = (y) => CX + spread(y) * SPREAD;

  const ys = $derived.by(() => {
    const out = [];
    for (let y = 0; y <= H; y += 3) out.push(y);
    return out;
  });
  const tapePath = (edge, outer) => {
    const inner = ys.map((y) => `${edge(y).toFixed(1)},${y}`);
    return `M${inner.join('L')}L${outer},${H}L${outer},0Z`;
  };
  const leftTape = $derived(tapePath(left, EDGE));
  const rightTape = $derived(tapePath(right, W - EDGE));
  const lining = $derived(
    'M' + ys.map((y) => `${left(y).toFixed(1)},${y}`).join('L') +
    'L' + [...ys].reverse().map((y) => `${right(y).toFixed(1)},${y}`).join('L') + 'Z'
  );
  const teeth = $derived.by(() => {
    const out = [];
    for (let i = 0, y = 4; y < H - 10; i++, y += PITCH) {
      const l = i % 2 === 0;
      out.push({ y, x: l ? left(y) - 2 : right(y) - 4 });
    }
    return out;
  });

  $effect(() => {
    const root = document.documentElement;
    const strap = document.querySelector('header');
    const foot = document.querySelector('footer');
    const measure = () => {
      docHeight = root.scrollHeight;
      top = strap ? strap.offsetHeight : 0;
      bottom = foot ? foot.offsetHeight : 0;
    };
    measure();
    document.documentElement.dataset.ready = '';
    const ro = new ResizeObserver(measure);
    ro.observe(root);
    if (strap) ro.observe(strap);
    if (foot) ro.observe(foot);
    return () => ro.disconnect();
  });

  let lastY = 0, vel = 0, angVel = 0, raf = 0;
  const still = () => matchMedia('(prefers-reduced-motion: reduce)').matches;
  function tick() {
    const y = window.scrollY;
    const v = y - lastY;
    lastY = y;
    vel = vel * 0.75 + v * 0.25;
    const target = Math.max(-24, Math.min(24, -vel * 0.8));
    angVel += (target - angle) * 0.14 - angVel * 0.2;
    angle += angVel;
    if (Math.abs(target) + Math.abs(angVel) + Math.abs(angle) > 0.05 || v !== 0) raf = requestAnimationFrame(tick);
    else { raf = 0; angle = 0; }
  }
  $effect(() => {
    scrollY;
    if (!raf && !still()) raf = requestAnimationFrame(tick);
  });
  $effect(() => () => cancelAnimationFrame(raf));

  function jump(y) {
    window.scrollTo({ top: ((y - 4) / travel) * range, behavior: 'instant' });
  }
  let grab = 0;
  function down(e) {
    if (e.button !== 0) return;
    e.preventDefault();
    const y = e.clientY - e.currentTarget.getBoundingClientRect().top;
    if (y >= sliderTop && y <= sliderTop + SLIDER) grab = y - sliderTop;
    else { grab = SLIDER / 2; jump(y - grab); }
    dragging = true;
    e.currentTarget.setPointerCapture(e.pointerId);
  }
  function move(e) {
    if (!dragging) return;
    jump(e.clientY - e.currentTarget.getBoundingClientRect().top - grab);
  }
  function up() { dragging = false; }
</script>

<svelte:window bind:scrollY bind:innerHeight />

{#if range > 0}
  <div
    class="zip"
    class:dragging
    aria-hidden="true"
    style:top="{top}px"
    style:bottom="{bottom}px"
    bind:clientHeight={H}
    onpointerdown={down}
    onpointermove={move}
    onpointerup={up}
    onpointercancel={up}
  >
    <svg width="100%" height={H} viewBox="0 0 {W} {H}" preserveAspectRatio="none">
      <path class="lining" d={lining} />
      <path class="tape" d={leftTape} />
      <path class="tape" d={rightTape} />
      <path class="stitch" d="M{EDGE + 3},0V{H}" />
      <path class="stitch" d="M{W - EDGE - 3},0V{H}" />
      {#each teeth as t (t.y)}
        <rect class="tooth" x={t.x.toFixed(1)} y={t.y} width="6" height="3" rx="1" />
      {/each}
      <rect class="stop" x={CX - 7} y={H - 10} width="14" height="7" rx="2" />
      <g class="slider" transform="translate({CX} {sliderTop})">
        <rect class="shade" x="-7" y="2" width="18" height="22" rx="4" />
        <rect class="body" x="-9" y="0" width="18" height="22" rx="4" />
        <g class="tab" transform="translate(0 {HINGE}) rotate({angle.toFixed(1)})">
          <rect x="-5" y="0" width="10" height={TAB} rx="3" />
          <circle class="hole" cx="0" cy={TAB - 7} r="2" />
        </g>
        <circle class="ring" cx="0" cy={HINGE} r="2.5" />
      </g>
    </svg>
  </div>
{/if}

<style>
  .zip {
    position: fixed; right: 0; bottom: 0; width: 30px; z-index: 39;
    touch-action: none; user-select: none;
    filter: drop-shadow(-1px 0 0 color-mix(in oklab, var(--shadow) 10%, transparent));
  }
  svg { display: block; }

  .lining { fill: var(--deep); }
  .tape   { fill: var(--sand); stroke: var(--line); stroke-width: 1.5; stroke-linejoin: round; }
  .stitch { fill: none; stroke: var(--line); stroke-width: 1.5; stroke-dasharray: 3 3; opacity: .28; }
  .tooth  { fill: var(--ink); }
  .stop   { fill: var(--ink); }

  .slider { cursor: grab; }
  .dragging .slider { cursor: grabbing; }
  .shade { fill: var(--shadow); }
  .body  { fill: var(--rust); stroke: var(--line); stroke-width: 2; transition: fill .15s ease; }
  .slider:hover .body, .dragging .body { fill: var(--rust-d); }
  .tab rect { fill: var(--mustard); stroke: var(--line); stroke-width: 2; }
  .hole  { fill: var(--canvas); stroke: var(--line); stroke-width: 1.5; }
  .ring  { fill: var(--ink); }

  @media (max-width: 640px) { .zip { width: 20px; } }
</style>
