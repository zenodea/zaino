<script>
  import { reveal } from '$lib/reveal.js';
  import Window from '$lib/components/Window.svelte';
</script>

<section class="band compartment">
  <div class="wrap" data-stagger use:reveal>
    <p class="kick">The plain REPL</p>
    <h2 class="big">Pipes and scripts<br />get a plainer zaino.</h2>
    <p class="body-copy">
      When stdin is not a terminal, zaino skips the interface and reads lines:
      each one a prompt, slash commands included. Answers go to stdout and
      everything else, tool calls, thinking, permission questions, goes to
      stderr, so whatever you redirect stays clean. <code>-p</code> asks one
      thing and exits, and <code>-json</code> wraps the answer with the usage
      and the cost for whatever is reading it.
    </p>
  </div>

  <div class="wrap shell" data-reveal use:reveal>
    <Window>
    <pre class="term"><span class="c"># answers on stdout, everything else on stderr</span>
<span class="p">$</span> git diff | zaino -p <span class="s">"review this"</span> -permission plan
<span class="p">$</span> zaino -p <span class="s">"what does install.sh do"</span> -json | jq -r .answer
<span class="p">$</span> echo <span class="s">"sum up this repo"</span> | zaino &gt; NOTES.md
<span class="p">$</span> zaino -v -max-spend 2 &lt; prompts.txt   <span class="c"># one turn per line, two dollars at most</span>
<span class="p">$</span> <span class="cur" aria-hidden="true">▌</span></pre>
    </Window>
  </div>
</section>

<style>
  .compartment { background: var(--sand); }

  .shell { margin-top: 34px; }
  .term {
    margin: 0; padding: 18px 24px 20px;
    font-size: 13px; line-height: 1.95;
    overflow-x: auto;
  }
  .term .p { color: var(--mustard); user-select: none; margin-right: 7px; }
  .term .c { color: #857a67; }
  .term .cur { color: var(--term-fg); animation: blink 1.15s steps(1) infinite; }
  @keyframes blink { 50% { opacity: 0; } }
  .term .s { color: #9dbf88; }
</style>
