<script>
  import { reveal } from '$lib/reveal.js';
</script>

<section class="band compartment">
  <div class="wrap two" data-stagger use:reveal>
    <div>
      <p class="kick">Providers</p>
      <h2 class="big">The loop doesn't<br />know the provider.</h2>
      <p class="body-copy">
        Only the packages under <code>internal/provider/</code> know a wire
        format. The loop and both frontends speak one neutral set of types and
        nothing else — so a third provider is one interface, and not a single
        branch anywhere else in the tree.
      </p>
      <p class="body-copy">
        No SDKs, either. Zaino speaks the Anthropic Messages API and the Gemini
        Generative Language API straight down the wire, over shared transport,
        retry and SSE framing. It finds whichever key you've got and starts.
      </p>
    </div>

    <div class="stack">
      <div class="tier tui lift"><span>tui</span><span>plain repl</span></div>
      <p class="seam">↕ <i>llm types only</i></p>
      <div class="tier loop lift">stream → run tools → repeat</div>
      <p class="seam">↕ <i>llm.Provider</i></p>
      <div class="tier prov lift">
        <span>anthropic</span><span>gemini</span><span class="empty">+ yours</span>
      </div>
    </div>
  </div>

  <div class="wrap" data-reveal use:reveal>
    <pre class="term"><span class="c"># it picks a provider out of the environment</span>
<span class="p">$</span> zaino
<span class="p">$</span> zaino -provider gemini -model gemini-2.5-flash
<span class="p">$</span> echo <span class="s">"say hi"</span> | zaino -v   <span class="c"># no terminal? line-based REPL</span>
<span class="p">$</span> <span class="cur" aria-hidden="true">▌</span></pre>
  </div>
</section>

<style>
  .compartment { background: var(--sand); border-top: 3px solid var(--ink); border-bottom: 3px solid var(--ink); }

  .stack { display: grid; gap: 0; }
  .tier {
    display: flex; gap: 8px; justify-content: center;
    background: var(--paper); border: 3px solid var(--ink); border-radius: 10px;
    box-shadow: 5px 5px 0 var(--shadow);
    padding: 15px 14px;
    font-family: var(--mono); font-size: 13px; text-align: center;
  }
  .tier:hover { box-shadow: 7px 7px 0 var(--shadow); }
  .tier span { flex: 1; }
  .tier.loop { background: var(--pkt); color: var(--on-pkt); font-weight: 700; }
  .tier.tui { transform: rotate(-.5deg); }
  .tier.prov { transform: rotate(.5deg); }
  .tier .empty { opacity: .5; border: 2px dashed currentColor; border-radius: 6px; }

  .seam {
    margin: 0; padding: 12px 0; text-align: center;
    font-family: var(--mono); font-size: 11.5px; color: var(--faint);
    letter-spacing: .06em;
  }
  .seam i { font-family: var(--disp); font-size: 14px; font-style: italic; color: var(--ink-2); }

  .term {
    margin: 46px 0 0;
    background: var(--term); color: var(--term-fg);
    border: 3px solid var(--ink); border-radius: 10px;
    box-shadow: 7px 7px 0 var(--shadow);
    padding: 22px 24px;
    font-family: var(--mono); font-size: 13px; line-height: 1.95;
    overflow-x: auto;
  }
  .term .p { color: var(--mustard); user-select: none; margin-right: 7px; }
  .term .c { color: #857a67; }
  .term .cur { color: var(--term-fg); animation: blink 1.15s steps(1) infinite; }
  @keyframes blink { 50% { opacity: 0; } }
  .term .s { color: #9dbf88; }
</style>
