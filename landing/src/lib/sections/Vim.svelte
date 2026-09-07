<script>
  import { onMount } from 'svelte';
  import { reveal } from '$lib/reveal.js';
  import Window from '$lib/components/Window.svelte';

  const frames = [];
  const push = (f, hold = 520) => frames.push({ mode: 'INSERT', sel: null, key: null, hold, ...f });

  let text = '';
  for (const ch of 'make the tets pass and commit') {
    text += ch;
    push({ text, cursor: text.length, key: null }, 55);
  }
  push({ text, cursor: text.length }, 700);
  push({ mode: 'NORMAL', text, cursor: text.length - 1, key: 'esc' }, 800);
  push({ mode: 'NORMAL', text, cursor: 0, key: '0' });
  push({ mode: 'NORMAL', text, cursor: 5, key: 'w' });
  push({ mode: 'NORMAL', text, cursor: 9, key: 'w' }, 650);
  text = 'make the  pass and commit';
  push({ text, cursor: 9, key: 'cw' }, 500);
  for (let k = 0; k < 'tests'.length; k++) {
    text = text.slice(0, 9 + k) + 'tests'[k] + text.slice(9 + k);
    push({ text, cursor: 10 + k }, 90);
  }
  push({ text, cursor: 14 }, 600);
  push({ mode: 'NORMAL', text, cursor: 13, key: 'esc' }, 700);
  push({ mode: 'NORMAL', text, cursor: text.length - 1, key: '$' });
  push({ mode: 'VISUAL', text, cursor: text.length - 1, sel: [text.length - 1, text.length], key: 'v' });
  push({ mode: 'VISUAL', text, cursor: 24, sel: [24, text.length], key: 'b' });
  push({ mode: 'VISUAL', text, cursor: 20, sel: [20, text.length], key: 'b' });
  push({ mode: 'VISUAL', text, cursor: 19, sel: [19, text.length], key: 'h' }, 750);
  text = 'make the tests pass';
  push({ mode: 'NORMAL', text, cursor: text.length - 1, key: 'd' }, 1100);
  push({ mode: 'SENT', text, cursor: -1, key: '⏎' }, 2200);

  let i = $state(0);
  let keys = $state([]);
  let playing = $state(false);
  let frame = $derived(frames[i]);

  function step() {
    if (!playing) return;
    i = (i + 1) % frames.length;
    const f = frames[i];
    if (i === 0) keys = [];
    if (f.key) keys = [...keys.slice(-7), { id: i, key: f.key }];
    setTimeout(step, f.hold);
  }

  function start() {
    if (playing) return;
    playing = true;
    setTimeout(step, 600);
  }

  onMount(() => () => (playing = false));
</script>

<section class="band vim" id="vim">
  <div class="wrap two">
    <div data-stagger use:reveal>
      <p class="kick">The composer</p>
      <h2 class="big">It speaks vim.</h2>
      <p class="body-copy">
        Not a plugin and not a keymap on top of a text box: the composer has a
        normal mode, a visual mode and an undo stack of its own. It starts in
        insert, so nothing is different until you press <kbd>esc</kbd>. Then
        <code>h j k l w b e 0 ^ $</code>, counts, <code>d</code> and
        <code>c</code> with a motion, <code>x</code>, <code>D</code>,
        <code>C</code>, <code>p</code>, <code>u</code>, and <code>v</code> or
        <code>V</code> to select. The footer shows the mode.
      </p>
      <p class="body-copy dim">
        The transcript keeps the same habits: <kbd>⌃j</kbd> and <kbd>⌃k</kbd>
        walk it, and every picker takes <kbd>j</kbd>, <kbd>k</kbd>,
        <kbd>g</kbd> and <kbd>G</kbd>. <code>-vim=false</code> gives you a plain
        text box instead.
      </p>
    </div>

    <div
      class="composer"
      data-reveal
      use:reveal={{ delay: 200, onreveal: start }}
    >
      <Window title="zaino · ~/work/app" tilt={-0.6}>

      <div class="line" aria-live="off">
        <span class="prompt">›</span>
        <span class="text">{#each frame.text.split('') as ch, n (n)}<span
            class:cursor={n === frame.cursor && frame.mode !== 'SENT'}
            class:sel={frame.sel && n >= frame.sel[0] && n < frame.sel[1]}
            class:sent={frame.mode === 'SENT'}>{ch === ' ' ? ' ' : ch}</span>{/each}{#if frame.cursor >= frame.text.length && frame.mode !== 'SENT'}<span class="cursor">&nbsp;</span>{/if}</span>
      </div>

      <div class="foot">
        <span class="mode" class:normal={frame.mode === 'NORMAL'} class:visual={frame.mode === 'VISUAL'} class:sent={frame.mode === 'SENT'}>
          {frame.mode === 'SENT' ? 'SENT' : frame.mode}
        </span>
        <span class="keys">
          {#each keys as { id, key } (id)}<kbd>{key}</kbd>{/each}
        </span>
        <span class="meta">claude-opus-5 · $0.14</span>
      </div>
      </Window>
    </div>
  </div>
</section>

<style>
  .vim { background: var(--canvas); }
  .vim .two { align-items: center; }

  .composer :global(.window) { font-size: 15px; }

  .line { display: flex; gap: 10px; padding: 26px 18px 22px; min-height: 84px; align-items: baseline; }
  .prompt { color: var(--mustard); font-weight: 700; }
  .text { line-height: 1.6; word-break: break-word; }
  .text span { border-radius: 2px; }
  .text .cursor { background: var(--term-fg); color: var(--term); }
  .text .sel { background: color-mix(in oklab, var(--rust) 55%, transparent); }
  .text .sent { color: #857a67; }

  .foot {
    display: flex; align-items: center; gap: 12px;
    padding: 9px 14px; border-top: 2px dashed color-mix(in oklab, var(--term-fg) 25%, transparent);
    font-size: 12px; color: #857a67;
  }
  .mode {
    font-weight: 700; letter-spacing: .1em; font-size: 11px;
    padding: 2px 9px; border-radius: 4px;
    background: #9dbf88; color: #1b1a15;
    transition: background .2s ease;
  }
  .mode.normal { background: var(--mustard); }
  .mode.visual { background: var(--rust); color: #fff8ee; }
  .mode.sent { background: #857a67; color: #1b1a15; }
  .keys { display: flex; gap: 5px; flex: 1; min-width: 0; overflow: hidden; }
  .keys kbd {
    font-size: 11px; padding: 1px 6px; border-radius: 4px;
    background: transparent; color: var(--term-fg);
    border: 1.5px solid #857a67; box-shadow: none;
    animation: key-in .25s ease both;
  }
  .meta { margin-left: auto; white-space: nowrap; }
  @keyframes key-in { from { opacity: 0; translate: 0 4px; } to { opacity: 1; translate: 0 0; } }
</style>
