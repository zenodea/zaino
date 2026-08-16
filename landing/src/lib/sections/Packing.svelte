<script>
  import { reveal } from '$lib/reveal.js';
</script>

<section class="band packing" id="pack">
  <div class="wrap" data-stagger use:reveal>
    <h2 class="big">What's built, and what isn't</h2>
    <p class="sub">
      Streaming, the turn loop, tools, permissions, both providers and the
      interface are implemented and tested.
    </p>

    <div class="lists">
      <div class="list packed lift">
        <h3><span class="chk">✔</span> Built</h3>
        <ul>
          <li>Streaming, and the turn loop that drives it</li>
          <li>Eight tools — <code>read write edit bash grep find ls fetch</code></li>
          <li><code>task</code>, which hands work to a second agent</li>
          <li>MCP servers, spoken over stdio</li>
          <li>Compaction when the context window fills</li>
          <li>Anthropic <span class="amp">&amp;</span> Gemini, hand-rolled</li>
          <li>Two frontends: full-screen TUI and a plain REPL</li>
          <li>A vim-style composer, and a walkable transcript</li>
          <li>Permissions, sessions, prompt recall</li>
          <li>Markdown rendering, wire logging</li>
        </ul>
        <p class="weigh">
          <b>276</b> tests, including the same tool round-trip driven through
          <i>each</i> provider, to prove the loop is provider-agnostic.
        </p>
      </div>

      <div class="list home lift">
        <h3><span class="chk out">✕</span> Not built</h3>
        <ul class="struck">
          <li>Branching</li>
        </ul>
        <p class="weigh">
          Every entry already carries an <code>id</code> and a
          <code>parent</code>, so the tree is what you get the first time the
          leaf is allowed to move backwards.
        </p>
      </div>
    </div>
  </div>
</section>

<style>
  .packing { background: var(--paper); border-top: 3px solid var(--ink); border-bottom: 3px solid var(--ink); }

  .lists { display: grid; grid-template-columns: 1.5fr 1fr; gap: 26px; }
  @media (max-width: 840px) { .lists { grid-template-columns: 1fr; } }

  .list {
    background: var(--canvas);
    border: 3px solid var(--ink);
    border-radius: 12px;
    box-shadow: 6px 6px 0 var(--shadow);
    padding: 26px 28px 22px;
  }
  .list:hover { box-shadow: 9px 9px 0 var(--shadow); }
  .list.home { transform: rotate(.6deg); }
  .list.packed { transform: rotate(-.4deg); }
  .list code { white-space: normal; }   /* the long tool list is allowed to wrap */

  .list h3 {
    font-size: 24px; margin-bottom: 16px;
    display: flex; align-items: center; gap: 10px;
  }
  .chk {
    display: inline-grid; place-items: center;
    width: 28px; height: 28px; flex: none;
    background: var(--forest); color: var(--canvas);
    border: 2.5px solid var(--ink); border-radius: 7px;
    font-family: var(--sans); font-size: 15px;
  }
  .chk.out { background: var(--rust); }

  .list ul { list-style: none; margin: 0 0 18px; padding: 0; }
  .list li {
    font-size: 15.5px; padding: 9px 0 9px 26px; position: relative;
    border-bottom: 2px dotted color-mix(in oklab, var(--ink) 28%, transparent);
  }
  .list li::before {
    content: "▪"; position: absolute; left: 4px; color: var(--rust);
  }
  .amp { font-family: var(--disp); font-style: italic; color: var(--rust); }
  .struck li { color: var(--faint); text-decoration: line-through; text-decoration-thickness: 2px; }
  .struck li::before { content: "✕"; font-size: 11px; color: var(--faint); text-decoration: none; }

  .weigh { font-size: 14.5px; color: var(--ink-2); margin: 0; }
  .weigh b {
    font-family: var(--disp); font-size: 26px; font-weight: 900;
    color: var(--rust); vertical-align: -2px;
  }
</style>
