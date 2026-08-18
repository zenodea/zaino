<script>
  import { repo } from '$lib/content.js';
  import Wordmark from './Wordmark.svelte';
</script>

<footer class="foot">
  <div class="wrap foot-in">
    <span>🎒 <b><Wordmark /></b> · an agent harness in Go</span>
    <span class="foot-links">
      <a href={repo}>GitHub</a>
      <a href="{repo}/issues">Issues</a>
      <span class="dim">MIT</span>
    </span>
  </div>
</footer>

<style>
  .foot { background: var(--ink); color: var(--canvas); padding: 26px 0; }
  .foot-in { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; font-size: 14px; }
  .foot-in b { font-family: var(--disp); font-weight: 800; }
  .foot-links { margin-left: auto; display: flex; gap: 18px; }
  .foot-links a { text-decoration: none; opacity: .8; }
  .foot-links a:hover { opacity: 1; text-decoration: underline; }
  .dim { opacity: .5; color: inherit; }
</style>
