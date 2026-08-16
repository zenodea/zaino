<script>
  import { reveal } from '$lib/reveal.js';
  import Backpack from '$lib/components/Backpack.svelte';
  import Install from '$lib/components/Install.svelte';
</script>

<section class="hero" id="top">
  <div class="wrap hero-grid">
    <div class="hero-copy" data-stagger use:reveal>
      <p class="sticker">
        🇮🇹 <b>zaino</b> <span class="ipa">/ˈdzai̯.no/</span> — <i>noun.</i> backpack
      </p>

      <h1>An agent harness<br />you can <em>carry.</em></h1>

      <p class="lede">
        Two provider clients written by hand, eight tools, subagents, MCP and a
        vim-style composer. It compiles to a single binary, and it asks before it
        writes or runs anything.
      </p>

      <Install />

      <p class="fine">
        Go 1.26.5+ · lands in <code>~/.local/bin</code> ·
        <code>--uninstall</code> takes it back out
      </p>
    </div>

    <Backpack delay={260} />
  </div>
</section>

<style>
  .hero { padding: 64px 0 40px; }
  .hero-grid { display: grid; grid-template-columns: 1.08fr 1fr; gap: 30px; align-items: center; }
  .hero-grid > :global(*) { min-width: 0; }   /* or they size to content and overflow */
  @media (max-width: 1000px) { .hero-grid { grid-template-columns: 1fr; gap: 20px; } }

  .sticker {
    display: inline-block;
    font-family: var(--mono); font-size: 12.5px;
    background: var(--mustard); color: #23201a;
    border: 2px solid var(--ink); border-radius: 6px;
    padding: 5px 12px 8px; margin-bottom: 26px;
    transform: rotate(-1.4deg);
    box-shadow: 3px 3px 0 var(--shadow);
  }
  .sticker:hover { animation: peel .5s ease-in-out; }
  @keyframes peel {
    30% { transform: rotate(.8deg) translateY(-2px); }
    65% { transform: rotate(-2.2deg); }
  }
  .sticker .ipa { opacity: .7; }
  .sticker b { font-weight: 700; }

  h1 {
    font-size: clamp(46px, 7.2vw, 82px);
    line-height: 0.96;
    font-variation-settings: "SOFT" 20, "WONK" 1;
    margin-bottom: 24px;
  }
  h1 em {
    font-style: italic; color: var(--rust);
    font-variation-settings: "SOFT" 60, "WONK" 1;
  }

  .lede { max-width: min(46ch, 100%); font-size: 18px; color: var(--ink-2); margin-bottom: 34px; }

  .fine { margin-top: 16px; font-size: 13px; color: var(--faint); font-family: var(--mono); }
  .fine code { font-size: .92em; }
</style>
