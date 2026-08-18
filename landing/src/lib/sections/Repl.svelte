<script>
  import { reveal } from '$lib/reveal.js';
</script>

<section class="band compartment">
  <div class="wrap" data-stagger use:reveal>
    <p class="kick">The plain REPL</p>
    <h2 class="big">A pipe is<br />a terminal too.</h2>
    <p class="body-copy">
      When stdin isn't a TTY, zaino skips the interface and reads lines: each
      one a prompt, slash commands included. Only the answer lands on stdout;
      the tool chatter, thinking and permission questions all go to stderr, so
      whatever you redirect stays clean.
    </p>
  </div>

  <div class="wrap" data-reveal use:reveal>
    <pre class="term"><span class="c"># answers on stdout, everything else on stderr</span>
<span class="p">$</span> echo <span class="s">"say hi"</span> | zaino
<span class="p">$</span> echo <span class="s">"sum up this repo"</span> | zaino &gt; NOTES.md
<span class="p">$</span> zaino -v &lt; prompts.txt   <span class="c"># one turn per line</span>
<span class="p">$</span> <span class="cur" aria-hidden="true">▌</span></pre>
  </div>
</section>

<style>
  .compartment { background: var(--sand); border-top: 3px solid var(--ink); border-bottom: 3px solid var(--ink); }

  .term {
    margin: 34px 0 0;
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
