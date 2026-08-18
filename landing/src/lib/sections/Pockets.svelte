<script>
  import { reveal } from '$lib/reveal.js';
  import { permissionModes } from '$lib/content.js';
</script>

<section class="band pockets" id="pockets">
  <div class="wrap" data-stagger use:reveal>
    <p class="kick">Four pockets</p>
    <h2 class="big">Permission is a mode.</h2>
    <p class="sub">
      <kbd>⇧⇥</kbd> cycles it mid-conversation; <code>-permission</code> sets it
      at the door.
    </p>

    <div class="pocket-row" data-stagger use:reveal>
      {#each permissionModes as { name, desc, stamp, warn, open } (name)}
        <div class="pkt lift" class:open>
          <span class="zipper"></span>
          <h3>{name}</h3>
          <p>{desc}</p>
          {#if stamp}
            <span class="stamp" class:warn>{stamp}</span>
          {/if}
        </div>
      {/each}
    </div>

    <div class="warnbox">
      <p>
        <b>One rule the modes can't lift.</b> Paths outside the working directory
        are <i>refused</i>, not asked about. Symlinks are resolved first, so a
        link pointing out of the tree is still out of the tree.
        <code>-allow-outside</code> is the only thing that moves it.
      </p>
    </div>
  </div>
</section>

<style>
  .pocket-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 18px; }
  @media (max-width: 920px) { .pocket-row { grid-template-columns: repeat(2, 1fr); } }
  @media (max-width: 520px) { .pocket-row { grid-template-columns: 1fr; } }

  .pkt {
    position: relative;
    background: var(--pkt); color: var(--on-pkt);
    border: 3px solid var(--ink);
    border-radius: 10px 10px 16px 16px;
    box-shadow: 5px 5px 0 var(--shadow);
    padding: 26px 18px 20px;
  }
  .pkt:hover { box-shadow: 7px 7px 0 var(--shadow); }
  .pkt:hover .zipper { opacity: 1; background-position-x: 4px; }
  .pkt:nth-child(2) { transform: rotate(-.7deg); }
  .pkt:nth-child(3) { transform: rotate(.5deg); }
  .pkt.open { background: var(--paper); color: var(--ink); border-style: dashed; }

  .zipper {
    position: absolute; left: 14px; right: 14px; top: 11px; height: 5px;
    border-radius: 3px;
    background:
      repeating-linear-gradient(90deg, var(--ink) 0 3px, transparent 3px 7px);
    opacity: .75;
    transition: opacity .25s ease, background-position .25s ease;
  }
  .pkt h3 { font-family: var(--mono); font-size: 15px; font-weight: 700; margin-bottom: 8px; }
  .pkt p  { font-size: 14.5px; margin: 0; opacity: .92; }

  .stamp {
    position: absolute; bottom: -11px; right: 12px;
    font-family: var(--mono); font-size: 10px; font-weight: 700;
    letter-spacing: .08em; text-transform: uppercase;
    background: var(--mustard); color: #23201a;
    border: 2px solid var(--ink); border-radius: 4px; padding: 1px 7px;
    transform: rotate(-3deg);
  }
  .stamp.warn { background: #d8352b; color: #fff; }

  .warnbox {
    margin: 44px 0 0;
    background: var(--mustard); color: #23201a;
    border: 3px solid var(--ink); border-radius: 10px;
    box-shadow: 6px 6px 0 var(--shadow);
    padding: 22px 26px;
  }
  .warnbox p { margin: 0; max-width: 78ch; font-size: 16.5px; }
  .warnbox code { background: rgba(255,255,255,.5); border-color: #23201a; }
</style>
