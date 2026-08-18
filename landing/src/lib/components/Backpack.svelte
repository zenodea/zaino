<script>
  import { packParts } from '$lib/content.js';
  import { reveal } from '$lib/reveal.js';

  let { delay = 0 } = $props();
</script>

<div class="pack-fig-wrap" data-reveal use:reveal={{ delay }}>
  <figure class="pack-fig">
    <svg
      class="pack"
      viewBox="0 86 760 410"
      role="img"
      aria-labelledby="packttl"
    >
      <title id="packttl">
        A backpack, with each part labelled as a part of zaino: the lid
        is five hand-rolled provider clients, the main compartment is the
        turn loop, the front pocket is eight tools, the buckle is
        permission, the side pocket is MCP servers, and the reinforced
        base is the test suite.
      </title>
  
      <path class="strapline" d="M300 200 C 250 250 245 360 275 430" />
      <path class="strapline" d="M460 200 C 510 250 515 360 485 430" />
  
      <path class="haul" d="M352 132 Q 380 96 408 132" />
  
      <rect
        class="body"
        x="262"
        y="150"
        width="236"
        height="330"
        rx="48"
      />
      <path
        class="base"
        d="M262 400 h236 v32 a48 48 0 0 1 -48 48 h-140 a48 48 0 0 1 -48 -48 z"
      />
  
      <path
        class="side"
        d="M262 300 h-26 a14 14 0 0 0 -14 14 v56 a14 14 0 0 0 14 14 h26 z"
      />
      <path
        class="side"
        d="M498 300 h26 a14 14 0 0 1 14 14 v56 a14 14 0 0 1 -14 14 h-26 z"
      />
  
      <rect
        class="lid"
        x="254"
        y="128"
        width="252"
        height="108"
        rx="42"
      />
      <path
        class="lidshade"
        d="M254 214 q126 26 252 0 v-14 a42 42 0 0 1 -42 36 h-168 a42 42 0 0 1 -42 -36 z"
      />
  
      <rect
        class="webbing"
        x="362"
        y="196"
        width="36"
        height="86"
        rx="8"
      />
      <rect
        class="buckle"
        x="352"
        y="222"
        width="56"
        height="34"
        rx="9"
      />
      <rect
        class="buckleslot"
        x="364"
        y="233"
        width="32"
        height="12"
        rx="6"
      />
  
      <rect
        class="pocket"
        x="292"
        y="308"
        width="176"
        height="112"
        rx="28"
      />
      <path class="zip" d="M304 322 h152" />
      <circle class="zippull" cx="456" cy="322" r="9" />
      <path class="zipline" d="M456 331 v14" />
  
      <circle class="patch" cx="380" cy="372" r="27" />
      <text class="patch-z" x="380" y="382">Z</text>
  
      <rect
        class="webbing sm"
        x="248"
        y="262"
        width="30"
        height="14"
        rx="7"
      />
      <rect
        class="webbing sm"
        x="482"
        y="262"
        width="30"
        height="14"
        rx="7"
      />
  
      <g class="callout">
        <path class="lead" pathLength="1" d="M196 152 H 250 L 276 168" />
        <circle class="dot" cx="276" cy="168" r="5" />
        <text class="part" x="188" y="140" text-anchor="end">
          THE LID
        </text>
        <text class="feat" x="188" y="164" text-anchor="end">
          five hand-rolled clients
        </text>
  
        <path class="lead" pathLength="1" d="M196 300 H 244 L 292 288" />
        <circle class="dot" cx="292" cy="288" r="5" />
        <text class="part" x="188" y="288" text-anchor="end">
          MAIN COMPARTMENT
        </text>
        <text class="feat" x="188" y="312" text-anchor="end">
          the turn loop
        </text>
  
        <path class="lead" pathLength="1" d="M196 442 H 262 L 306 400" />
        <circle class="dot" cx="306" cy="400" r="5" />
        <text class="part" x="188" y="430" text-anchor="end">
          FRONT POCKET
        </text>
        <text class="feat" x="188" y="454" text-anchor="end">
          eight tools
        </text>
  
        <path class="lead" pathLength="1" d="M564 176 H 520 L 468 186" />
        <circle class="dot" cx="468" cy="186" r="5" />
        <text class="part" x="572" y="164">HAUL LOOP</text>
        <text class="feat" x="572" y="188">one binary, no runtime</text>
  
        <path class="lead" pathLength="1" d="M564 258 H 470 L 412 240" />
        <circle class="dot" cx="412" cy="240" r="5" />
        <text class="part" x="572" y="246">THE BUCKLE</text>
        <text class="feat" x="572" y="270">asks before it acts</text>
  
        <path class="lead" pathLength="1" d="M564 344 H 546" />
        <circle class="dot" cx="524" cy="344" r="5" />
        <text class="part" x="572" y="332">SIDE POCKET</text>
        <text class="feat" x="572" y="356">MCP, over stdio</text>
  
        <path class="lead" pathLength="1" d="M564 452 H 512 L 466 466" />
        <circle class="dot" cx="466" cy="466" r="5" />
        <text class="part" x="572" y="440">REINFORCED BASE</text>
        <text class="feat" x="572" y="464">tested throughout</text>
      </g>
    </svg>
    <figcaption>fig. 1: the pack, from the outside</figcaption>
  </figure>

  <!-- Same labels as the leader lines, for when the callouts are hidden. -->
  <ul class="pack-key">
    {#each packParts as { part, feat } (part)}
      <li><b>{part}</b> {feat}</li>
    {/each}
  </ul>
</div>

<style>
  .pack-fig-wrap, .pack-fig { margin: 0; }
  .pack { width: 100%; height: auto; overflow: visible; }
  .pack-fig figcaption {
    text-align: center; margin-top: 6px;
    font-family: var(--mono); font-size: 11.5px; letter-spacing: .1em;
    text-transform: uppercase; color: var(--faint);
  }

  .pack .body      { fill: var(--rust); stroke: var(--ink); stroke-width: 3.5; }
  .pack .base      { fill: var(--rust-d); stroke: var(--ink); stroke-width: 3.5; }
  .pack .lid       { fill: var(--rust-d); stroke: var(--ink); stroke-width: 3.5; }
  .pack .lidshade  { fill: rgba(0,0,0,.14); stroke: none; }
  .pack .pocket    { fill: var(--pocket); stroke: var(--ink); stroke-width: 3.5; }
  .pack .side      { fill: var(--rust-d); stroke: var(--ink); stroke-width: 3.5; }
  .pack .webbing   { fill: var(--forest); stroke: var(--ink); stroke-width: 3; }
  .pack .buckle    { fill: var(--mustard); stroke: var(--ink); stroke-width: 3; }
  .pack .buckleslot{ fill: var(--rust-d); stroke: none; }
  .pack .haul      { fill: none; stroke: var(--forest); stroke-width: 12; stroke-linecap: round; }
  .pack .strapline { fill: none; stroke: var(--forest); stroke-width: 26; stroke-linecap: round; }
  .pack .zip       { stroke: var(--ink); stroke-width: 3; stroke-dasharray: 2 6; stroke-linecap: round; }
  .pack .zippull   { fill: var(--mustard); stroke: var(--ink); stroke-width: 3; }
  .pack .zipline   { stroke: var(--ink); stroke-width: 3; stroke-linecap: round; }
  .pack .patch     { fill: var(--mustard); stroke: var(--ink); stroke-width: 3.5; }
  .pack .patch-z   {
    font-family: var(--disp); font-weight: 900; font-size: 30px;
    fill: #23201a; text-anchor: middle;
  }

  .pack .lead { fill: none; stroke: var(--ink); stroke-width: 2.5; opacity: .45; }
  .pack .dot  { fill: var(--ink); }
  .pack .part {
    font-family: var(--mono); font-size: 14.5px; font-weight: 700;
    letter-spacing: .08em; fill: var(--faint);
  }
  .pack .feat {
    font-family: var(--disp); font-weight: 700; font-size: 27px;
    font-style: italic; fill: var(--ink);
  }

  /* The pack lands, then labels itself: each leader line draws, its dot lands,
     and the words fade in behind it. --step is that callout's place in the
     queue, counted clockwise from the lid. */
  .pack .lead { stroke-dasharray: 1; }

  .callout path:nth-of-type(1), .callout circle:nth-of-type(1),
  .callout text:nth-of-type(1), .callout text:nth-of-type(2) { --step: 0; }
  .callout path:nth-of-type(2), .callout circle:nth-of-type(2),
  .callout text:nth-of-type(3), .callout text:nth-of-type(4) { --step: 1; }
  .callout path:nth-of-type(3), .callout circle:nth-of-type(3),
  .callout text:nth-of-type(5), .callout text:nth-of-type(6) { --step: 2; }
  .callout path:nth-of-type(4), .callout circle:nth-of-type(4),
  .callout text:nth-of-type(7), .callout text:nth-of-type(8) { --step: 3; }
  .callout path:nth-of-type(5), .callout circle:nth-of-type(5),
  .callout text:nth-of-type(9), .callout text:nth-of-type(10) { --step: 4; }
  .callout path:nth-of-type(6), .callout circle:nth-of-type(6),
  .callout text:nth-of-type(11), .callout text:nth-of-type(12) { --step: 5; }
  .callout path:nth-of-type(7), .callout circle:nth-of-type(7),
  .callout text:nth-of-type(13), .callout text:nth-of-type(14) { --step: 6; }

  :root[data-js] .pack-fig-wrap:not([data-revealed]) .lead { stroke-dashoffset: 1; }
  :root[data-js] .pack-fig-wrap:not([data-revealed]) .dot,
  :root[data-js] .pack-fig-wrap:not([data-revealed]) .part,
  :root[data-js] .pack-fig-wrap:not([data-revealed]) .feat,
  :root[data-js] .pack-fig-wrap:not([data-revealed]) .patch,
  :root[data-js] .pack-fig-wrap:not([data-revealed]) .patch-z { opacity: 0; }

  .pack-fig-wrap:global([data-revealed]) .lead {
    animation: draw .5s ease-out backwards;
    animation-delay: calc(.35s + var(--step) * .08s);
  }
  .pack-fig-wrap:global([data-revealed]) .dot {
    animation: land .35s cubic-bezier(.3,1.6,.5,1) backwards;
    animation-delay: calc(.6s + var(--step) * .08s);
  }
  .pack-fig-wrap:global([data-revealed]) .part,
  .pack-fig-wrap:global([data-revealed]) .feat {
    animation: name .45s ease-out backwards;
    animation-delay: calc(.66s + var(--step) * .08s);
  }
  .pack-fig-wrap:global([data-revealed]) .patch,
  .pack-fig-wrap:global([data-revealed]) .patch-z {
    animation: sew .5s cubic-bezier(.3,1.5,.5,1) .3s backwards;
  }
  .pack-fig-wrap:global([data-revealed]) .zippull,
  .pack-fig-wrap:global([data-revealed]) .zipline {
    animation: swing 1.4s ease-in-out .75s 1;
  }

  .pack .dot, .pack .patch, .pack .patch-z { transform-box: fill-box; transform-origin: center; }
  .pack .zippull, .pack .zipline { transform-box: view-box; transform-origin: 456px 322px; }

  @keyframes draw { from { stroke-dashoffset: 1; } }
  @keyframes land { from { opacity: 0; scale: 0; } }
  @keyframes name { from { opacity: 0; translate: 0 5px; } }
  @keyframes sew  { from { opacity: 0; scale: .3; rotate: -22deg; } }
  @keyframes swing {
    30% { rotate: 7deg; }
    60% { rotate: -5deg; }
    85% { rotate: 2deg; }
  }

  .pack-key { display: none; }

  /* Too narrow for leader lines: the pack sits dead centre of the viewBox, so
     scaling up and cropping to it is symmetric. Parts are listed underneath. */
  @media (max-width: 700px) {
    .pack-fig { overflow: hidden; }
    .pack { width: 150%; margin-left: -25%; }
    .pack .callout { display: none; }

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
