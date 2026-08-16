<script>
  import { reveal } from '$lib/reveal.js';
  import { installCommand, repo } from '$lib/content.js';
  import Cmd from '$lib/components/Cmd.svelte';
</script>

<section class="cta">
  <div class="wrap" data-stagger use:reveal>
    <p class="cta-mark">🎒</p>
    <h2>Pick it up.</h2>
    <Cmd cmd={installCommand} solo />
    <p class="cta-links">
      <a class="btn" href={repo}>Read the source</a>
      <a class="btn alt" href="{repo}#readme">Documentation</a>
    </p>
  </div>
</section>

<style>
  .cta {
    padding: 90px 0 96px; text-align: center;
    background: var(--deep); color: var(--on-deep);
    border-top: 3px solid var(--ink);
  }
  .cta-mark {
    font-size: 54px; margin: 0 0 6px; transform: rotate(-6deg);
    display: inline-block; cursor: default;
  }
  .cta-mark:hover { animation: shoulder .6s ease-in-out; }
  @keyframes shoulder {
    35% { translate: 0 -10px; rotate: 8deg; }
    70% { translate: 0 0; rotate: -3deg; }
  }
  .cta h2 {
    font-size: clamp(44px, 7vw, 76px); line-height: 1;
    font-variation-settings: "SOFT" 40, "WONK" 1;
    margin-bottom: 30px;
  }
  .cta-links { display: flex; gap: 14px; justify-content: center; margin: 32px 0 0; flex-wrap: wrap; }
  .btn {
    font-family: var(--mono); font-size: 14px; font-weight: 700;
    text-decoration: none; padding: 11px 22px; border-radius: 8px;
    background: var(--mustard); color: #23201a;
    border: 3px solid var(--ink); box-shadow: 5px 5px 0 rgba(0,0,0,.45);
    transition: translate .12s ease, box-shadow .12s ease;
  }
  .btn:hover, .btn:active { translate: 2px 2px; box-shadow: 3px 3px 0 rgba(0,0,0,.45); }
  .btn.alt { background: transparent; color: var(--on-deep); border-color: var(--on-deep); box-shadow: 5px 5px 0 rgba(0,0,0,.35); }
</style>
