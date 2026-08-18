<script>
  import { nav, repo } from '$lib/content.js';
  import { theme, cycleTheme } from '$lib/theme.svelte.js';
  import Wordmark from './Wordmark.svelte';

  let scrollY = $state(0);
</script>

<svelte:window bind:scrollY />

<header class="strap" class:carried={scrollY > 12}>
  <div class="wrap strap-in">
    <a class="brand" href="#top">
      <span class="brand-mark">🎒</span>
      <Wordmark />
    </a>
    <nav class="strap-links">
      {#each nav as { href, label } (href)}
        <a {href}>{label}</a>
      {/each}
    </nav>
    <a class="patch-link" href={repo}>github ↗</a>
    <button class="theme" type="button" aria-label="Change theme" onclick={cycleTheme}>
      {theme.mode}
    </button>
  </div>
  <span class="webbing" aria-hidden="true"></span>
</header>

<style>
  .strap {
    position: sticky; top: 0; z-index: 40;
    background: var(--deep);
    border-bottom: 3px solid var(--ink);
    color: var(--on-deep);
    transition: box-shadow .25s ease;
  }
  .strap.carried { box-shadow: 0 5px 0 color-mix(in oklab, var(--shadow) 20%, transparent); }

  /* How far down the page you are, drawn as a strap being pulled through. */
  .webbing {
    position: absolute; left: 0; right: 0; bottom: -3px; height: 3px;
    background: var(--mustard); transform-origin: 0 50%; display: none;
  }
  @supports (animation-timeline: scroll()) {
    .webbing {
      display: block;
      animation: pull linear;
      animation-timeline: scroll(root block);
    }
  }
  @keyframes pull { from { transform: scaleX(0); } to { transform: scaleX(1); } }
  /* A scroll timeline has no duration to shorten, so it goes rather than crawls. */
  @media (prefers-reduced-motion: reduce) { .webbing { display: none; } }

  .strap-in { display: flex; align-items: center; gap: 22px; height: 58px; }

  .brand {
    display: inline-flex; align-items: center; gap: 8px;
    font-family: var(--disp); font-weight: 900; font-size: 21px;
    text-decoration: none; letter-spacing: -0.02em; flex: none;
  }
  .brand-mark {
    font-size: 19px; display: inline-block;
    rotate: -8deg;
  }

  .strap-links { margin-left: auto; display: flex; gap: 20px; font-size: 14px; }
  .strap-links a {
    position: relative; text-decoration: none; opacity: .78;
    transition: opacity .2s ease;
  }
  .strap-links a::after {
    content: ""; position: absolute; left: 0; right: 0; bottom: -5px; height: 2px;
    background: var(--mustard); transform: scaleX(0); transform-origin: 0 50%;
    transition: transform .25s cubic-bezier(.2,.6,.2,1);
  }
  .strap-links a:hover { opacity: 1; }
  .strap-links a:hover::after { transform: scaleX(1); }

  .patch-link {
    flex: none; font-family: var(--mono); font-size: 12px; font-weight: 700;
    background: var(--mustard); color: #23201a; text-decoration: none;
    border: 2px solid #23201a; border-radius: 20px; padding: 3px 12px;
    box-shadow: 2px 2px 0 #23201a;
    transition: translate .12s ease, box-shadow .12s ease;
  }
  .patch-link:hover, .patch-link:active { translate: 1px 1px; box-shadow: 1px 1px 0 #23201a; }

  .theme {
    flex: none; cursor: pointer;
    font-family: var(--mono); font-size: 11px; color: inherit;
    background: transparent; border: 2px solid currentColor; opacity: .6;
    border-radius: 20px; padding: 3px 10px;
    transition: opacity .2s ease, translate .12s ease;
  }
  .theme:hover { opacity: 1; }
  .theme:active { translate: 1px 1px; }

  @media (max-width: 940px) { .strap-links { display: none; } .strap-in { gap: 12px; } }
</style>
