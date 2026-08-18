<script>
  import { reveal } from '$lib/reveal.js';

  let shown = $state(false);
</script>

<section class="band journey" id="journey">
  <div class="wrap two roomy">
    <div data-stagger use:reveal>
      <p class="kick">The journey</p>
      <h2 class="big">Nothing you tried<br />is lost.</h2>
      <p class="body-copy">
        <code>/rewind</code> takes the conversation up again from an earlier
        turn: your prompt comes back to the composer to be changed and asked
        again, and everything after it leaves the context. Nothing is deleted:
        the turns you walk away from stay in the file, on a branch of their
        own. The file has always been a tree.
      </p>
      <p class="body-copy dim">
        <code>/journey</code> draws that file whole: every turn on every road,
        the abandoned ones dimmed. Pick any stop, lit or not, and the context
        is rebuilt as it stood there. A rewind walks back along the road you
        are on; a journey can cross to one you left.
      </p>
    </div>

    <div
      class="map lift"
      class:shown
      data-reveal
      use:reveal={{ delay: 200, onreveal: () => (shown = true) }}
    >
      <p class="map-top">TRAIL MAP</p>

      <svg viewBox="0 0 440 460" role="img"
        aria-label="A conversation drawn as a trail: the taken road solid, two abandoned branches dashed.">

        <!-- Roads left behind: the oldest attempt keeps its lane. -->
        <path class="road left leftA" d="M 60 170 L 60 365" />
        <path class="road left leftB" d="M 180 342 L 180 430" />

        <!-- The taken road curves out of every fork. -->
        <path class="road main" pathLength="1"
          d="M 60 30 L 60 170 C 60 215 180 200 180 245 L 180 315 C 180 365 300 350 300 400" />

        <!-- Stops on the taken road -->
        <circle class="stop on s1" cx="60" cy="30" r="8" />
        <circle class="stop on s2" cx="60" cy="100" r="8" />
        <circle class="stop on s3" cx="60" cy="170" r="8" />
        <circle class="stop on s4" cx="180" cy="245" r="8" />
        <circle class="stop on s5" cx="180" cy="315" r="8" />

        <!-- Stops left behind -->
        <circle class="stop off a1" cx="60" cy="262" r="7" />
        <circle class="stop off a2" cx="60" cy="365" r="7" />
        <circle class="stop off b1" cx="180" cy="430" r="7" />

        <!-- You are here -->
        <circle class="pulse s6" cx="300" cy="400" r="8" />
        <circle class="ring s6" cx="300" cy="400" r="13" />
        <circle class="stop on s6" cx="300" cy="400" r="8" />

        <!-- What was asked, stop by stop -->
        <text class="mark s1" x="78" y="35">scaffold the page</text>
        <text class="mark s2" x="78" y="105">add a nav</text>
        <text class="mark s3" x="78" y="175">style the hero</text>
        <text class="mark faded a1" x="78" y="267">try floats</text>
        <text class="mark faded a2" x="78" y="370">clear floats</text>
        <text class="mark s4" x="198" y="250">use flexbox</text>
        <text class="mark s5" x="198" y="320">add the footer</text>
        <text class="mark faded b1" x="198" y="435">center with margins</text>
        <text class="mark s6" x="322" y="405">use grid</text>

      </svg>

      <p class="map-bot">THE OLDEST ATTEMPT KEEPS ITS LANE</p>
    </div>
  </div>
</section>

<style>
  .journey {
    background: var(--sand);
    border-top: 3px solid var(--ink);
    border-bottom: 3px solid var(--ink);
  }

  /* The map gets the wider half, so its labels stay legible. */
  .roomy { grid-template-columns: 0.8fr 1.2fr; }
  @media (max-width: 900px) { .roomy { grid-template-columns: 1fr; } }

  .map {
    background: var(--canvas);
    border: 3px solid var(--ink);
    box-shadow: 6px 6px 0 var(--shadow);
    border-radius: 4px;
    padding: 18px 24px 14px;
    transform: rotate(-1deg);
    font-family: var(--mono);
  }
  .map:hover { box-shadow: 9px 9px 0 var(--shadow); }

  .map-top, .map-bot {
    text-align: center; font-size: 10.5px; font-weight: 700;
    letter-spacing: .16em; color: var(--faint); margin: 0;
  }
  .map-top { border-bottom: 2px dashed var(--ink); padding-bottom: 10px; }
  .map-bot { border-top: 2px dashed var(--ink); padding-top: 10px; }

  svg { display: block; width: 100%; height: auto; margin: 8px 0 4px; }

  /* Base styles are the finished map, so it is whole without script; the
     animations replay the journey when the card scrolls into view, their
     `both` fill holding each piece unseen until the road reaches it. */
  .road { fill: none; stroke-linecap: round; }
  .road.main { stroke: var(--rust); stroke-width: 5.5; stroke-dasharray: 1; }
  .map.shown .road.main {
    animation: draw 1.8s cubic-bezier(.45,.05,.55,.95) both;
  }

  /* The roads left behind: thin, dashed, and still going somewhere. */
  .road.left { stroke: var(--faint); stroke-width: 3; stroke-dasharray: 4 8; }
  .map.shown .leftA { animation: appear .5s ease both .8s, march 1.4s linear infinite; }
  .map.shown .leftB { animation: appear .5s ease both 1.45s, march 1.4s linear infinite; }

  .stop {
    stroke: var(--ink); stroke-width: 2.5;
    transform-box: fill-box; transform-origin: center;
  }
  .stop.on  { fill: var(--rust); }
  .stop.off { fill: var(--canvas); stroke: var(--faint); }

  .mark { font-family: var(--mono); font-size: 13.5px; fill: var(--ink-2); }
  .mark.faded { fill: var(--faint); }

  .ring { fill: none; stroke: var(--rust); stroke-width: 2.5; }
  .pulse { fill: none; stroke: var(--rust); stroke-width: 2; opacity: 0; }

  /* Everything pops in as the road reaches it. */
  .map.shown .stop { animation: pop .35s cubic-bezier(.2,.9,.3,1.4) both; }
  .map.shown .mark { animation: appear .45s ease both; }
  .map.shown .ring { animation: appear .4s ease both 1.9s; }
  .map.shown .pulse {
    transform-box: fill-box; transform-origin: center;
    animation: pulse 2.4s ease-out infinite 2.2s;
  }
  .map.shown .s1 { animation-delay: .1s; }
  .map.shown .s2 { animation-delay: .4s; }
  .map.shown .s3 { animation-delay: .7s; }
  .map.shown .s4 { animation-delay: 1.1s; }
  .map.shown .s5 { animation-delay: 1.35s; }
  .map.shown .a1 { animation-delay: 1s; }
  .map.shown .a2 { animation-delay: 1.2s; }
  .map.shown .b1 { animation-delay: 1.65s; }
  .map.shown .stop.s6 { animation-delay: 1.75s; }
  .map.shown .mark.s6 { animation-delay: 1.75s; }

  @keyframes draw {
    from { stroke-dashoffset: 1; }
    to   { stroke-dashoffset: 0; }
  }
  @keyframes march  { to { stroke-dashoffset: -12; } }
  @keyframes appear {
    from { opacity: 0; }
    to   { opacity: 1; }
  }
  @keyframes pop {
    from { opacity: 0; scale: 0.3; }
    to   { opacity: 1; scale: 1; }
  }
  @keyframes pulse {
    0%   { opacity: .8; scale: 1; }
    70%  { opacity: 0;  scale: 2.2; }
    100% { opacity: 0;  scale: 2.2; }
  }
</style>
