<script>
  import { reveal } from '$lib/reveal.js';
  import Window from '$lib/components/Window.svelte';

  let shown = $state(false);
</script>

<section class="band hooks" id="hooks">
  <div class="wrap" data-stagger use:reveal>
    <p class="kick">Hooks</p>
    <h2 class="big">Your scripts,<br />inside the loop.</h2>
    <p class="sub">
      Four places to run something of your own: when a session starts, before
      and after every tool call, and when a turn ends. A <code>pre-tool</code>
      script that exits 2 blocks the call, and its stderr is what the model
      reads instead of a result. Whatever a <code>post-tool</code> script prints
      goes on the end of the result, so a formatter or a linter gets a say after
      every edit, and the model fixes what it sees.
    </p>
  </div>

  <div class="wrap">
    <div
      class="loop lift"
      class:shown
      data-reveal
      use:reveal={{ delay: 150, onreveal: () => (shown = true) }}
    >
      <p class="loop-top">THE TURN LOOP</p>

      <svg viewBox="0 0 760 360" role="img"
        aria-label="The turn loop drawn as a road: a prompt reaches the model, the model calls a tool, the result goes back to the model, round again until it answers. Hook stations sit at the start, before and after the tool, and at the answer.">
        <path class="road in" pathLength="1" d="M 120 46 L 120 93" />

        <path class="road main" pathLength="1"
          d="M 120 130 L 250 130 L 470 130 L 640 130 C 680 130 690 145 690 175 L 690 205 C 690 240 675 250 640 250 L 170 250 C 135 250 120 235 120 200 L 120 167" />

        <polygon class="arrow arrow-loop" points="120,154 111,168 129,168" />
        <polygon class="arrow arrow-in" points="120,106 111,92 129,92" />
        <polygon class="arrow arrow-out" points="52,276 43,262 61,262" />

        <path class="road out" pathLength="1" d="M 109 130 C 72 130 52 152 52 195 L 52 263" />

        <circle class="packet" r="6">
          <animateMotion dur="6.5s" repeatCount="indefinite" begin="2.5s"
            path="M 120 130 L 250 130 L 470 130 L 640 130 C 680 130 690 145 690 175 L 690 205 C 690 240 675 250 640 250 L 170 250 C 135 250 120 235 120 200 L 120 152 L 120 130" />
        </circle>

        <circle class="stop s-model" cx="120" cy="130" r="11" />
        <circle class="stop s-tool" cx="470" cy="130" r="11" />
        <circle class="stop s-answer" cx="52" cy="300" r="10" />

        <text class="mark s-model" x="140" y="120">the model</text>
        <text class="mark s-tool" x="470" y="106" text-anchor="middle">the tool runs</text>
        <text class="mark s-answer" x="70" y="305">the answer</text>

        <g class="station st-start">
          <rect x="45" y="10" width="150" height="34" rx="7" />
          <text x="120" y="32">session-start</text>
        </g>
        <g class="station st-pre">
          <rect x="250" y="113" width="120" height="34" rx="7" />
          <text x="310" y="135">pre-tool</text>
        </g>
        <g class="station st-post">
          <rect x="560" y="113" width="126" height="34" rx="7" />
          <text x="623" y="135">post-tool</text>
        </g>
        <g class="station st-end">
          <rect x="190" y="284" width="120" height="34" rx="7" />
          <text x="250" y="306">turn-end</text>
        </g>

      </svg>

      <p class="loop-bot">THE YELLOW STOPS ARE YOUR SCRIPTS</p>
    </div>

    <div class="two below" data-stagger use:reveal>
      <Window title="~/.config/zaino/config.json">
      <pre class="term">&#123;
  <span class="k">"hooks"</span>: &#123;
    <span class="k">"session-start"</span>: [&#123;<span class="k">"run"</span>: <span class="s">"git fetch -q"</span>&#125;],
    <span class="k">"pre-tool"</span>:      [&#123;<span class="k">"tool"</span>: <span class="s">"bash"</span>, <span class="k">"run"</span>: <span class="s">"sh .zaino/guard.sh"</span>&#125;],
    <span class="k">"post-tool"</span>:     [&#123;<span class="k">"tool"</span>: <span class="s">"edit,write"</span>, <span class="k">"run"</span>: <span class="s">"gofmt -l . 2>&amp;1"</span>&#125;],
    <span class="k">"turn-end"</span>:      [&#123;<span class="k">"run"</span>: <span class="s">"notify-send zaino done"</span>&#125;]
  &#125;
&#125;</pre>
      </Window>
      <p class="body-copy dim">
        Declared per event, in either config file, with <code>tool</code> to
        narrow one to some tools. Each runs with <code>sh -c</code> in the
        project directory. The tool, its input and, afterwards, its result
        arrive as JSON on stdin; <code>ZAINO_EVENT</code> and
        <code>ZAINO_TOOL</code> are in the environment for one-liners. Exit 0
        lets it through, exit 2 says no, anything else is reported to you and
        ignored. Subagents run the same hooks. <code>/hooks</code> lists what is
        set up.
      </p>
    </div>
  </div>
</section>

<style>
  .hooks {
    background: var(--paper);
  }
  .hooks .sub { max-width: 66ch; }


  .loop {
    max-width: 900px; margin: 8px auto 0;
    background: var(--canvas);
    border: 3px solid var(--line);
    box-shadow: 6px 6px 0 var(--shadow);
    border-radius: 4px;
    padding: 18px 20px 14px;
    transform: rotate(.6deg);
    font-family: var(--mono);
  }
  .loop:hover { box-shadow: 9px 9px 0 var(--shadow); }

  .loop-top, .loop-bot {
    text-align: center; font-size: 10.5px; font-weight: 700;
    letter-spacing: .16em; color: var(--faint); margin: 0;
  }
  .loop-top { border-bottom: 2px dashed var(--line); padding-bottom: 10px; }
  .loop-bot { border-top: 2px dashed var(--line); padding-top: 10px; }

  svg { display: block; width: 100%; height: auto; margin: 10px 0 6px; overflow: visible; }

  .road { fill: none; stroke-linecap: round; stroke-linejoin: round; }
  .road.main { stroke: var(--rust); stroke-width: 5; stroke-dasharray: 1; }
  .road.out, .road.in { stroke: var(--faint); stroke-width: 3; stroke-dasharray: 1; }
  .arrow { fill: var(--rust); }
  .arrow-in, .arrow-out { fill: var(--faint); }
  .loop.shown .road.in   { animation: draw .5s ease both .1s; }
  .loop.shown .road.main { animation: draw 1.6s cubic-bezier(.45,.05,.55,.95) both .5s; }
  .loop.shown .road.out  { animation: draw .5s ease both 1.9s; }

  .packet { fill: var(--mustard); stroke: var(--line); stroke-width: 2; opacity: 0; }
  .loop.shown .packet { animation: appear .3s ease both 2.5s; }

  .stop { fill: var(--rust); stroke: var(--line); stroke-width: 2.5; transform-box: fill-box; transform-origin: center; }
  .stop.s-back { fill: var(--canvas); stroke: var(--faint); }
  .stop.s-answer { fill: var(--canvas); stroke: var(--line); }

  .mark { font-family: var(--mono); font-size: 14px; fill: var(--ink-2); }
  .mark.faded { fill: var(--faint); }

  .station rect { fill: var(--mustard); stroke: var(--line); stroke-width: 2.5; }
  .station text {
    font-family: var(--mono); font-size: 13px; font-weight: 700;
    fill: #23201a; text-anchor: middle;
  }
  .note { font-family: var(--mono); font-size: 12px; fill: var(--faint); text-anchor: middle; }

  .loop.shown .arrow-in   { animation: appear .2s ease both .6s; }
  .loop.shown .arrow-loop { animation: appear .3s ease both 2.1s; }
  .loop.shown .arrow-out  { animation: appear .2s ease both 2.4s; }
  .loop.shown .stop { animation: pop .35s cubic-bezier(.2,.9,.3,1.4) both; }
  .loop.shown .station, .loop.shown .mark, .loop.shown .note { animation: appear .5s ease both; }
  .loop.shown .st-start { animation-delay: .1s; }
  .loop.shown .s-model  { animation-delay: .6s; }
  .loop.shown .st-pre   { animation-delay: .9s; }
  .loop.shown .s-tool   { animation-delay: 1.1s; }
  .loop.shown .st-post  { animation-delay: 1.35s; }
  .loop.shown .s-back   { animation-delay: 1.7s; }
  .loop.shown .s-answer { animation-delay: 2.2s; }
  .loop.shown .st-end   { animation-delay: 2.4s; }

  @keyframes draw { from { stroke-dashoffset: 1; } to { stroke-dashoffset: 0; } }
  @keyframes appear { from { opacity: 0; } to { opacity: 1; } }
  @keyframes pop { from { opacity: 0; scale: 0.3; } to { opacity: 1; scale: 1; } }
  @media (prefers-reduced-motion: reduce) {
    .loop.shown * { animation: none !important; }
    .packet { opacity: 1; }
  }

  .below { max-width: 900px; margin: 34px auto 0; grid-template-columns: 1fr; gap: 22px; }
  .term {
    margin: 0; padding: 16px 20px 18px;
    font-size: 13px; line-height: 1.75;
    overflow-x: auto;
  }
  .term .c { color: #857a67; }
  .term .k { color: var(--mustard); }
  .term .s { color: #9dbf88; }
</style>
