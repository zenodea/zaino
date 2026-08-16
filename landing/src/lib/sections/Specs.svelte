<script>
  import { reveal } from '$lib/reveal.js';
</script>

<section class="band specs" id="specs">
  <div class="wrap" data-stagger use:reveal>
    <h2 class="big">Specifications</h2>
    <dl class="spec" data-stagger use:reveal>
      <div>
        <dt>Weight</dt>
        <dd>One binary. No runtime to install, no daemon to keep running.</dd>
      </div>
      <div>
        <dt>Material</dt>
        <dd>Go 1.26.5, and Bubble Tea for the interface.</dd>
      </div>
      <div>
        <dt>Capacity</dt>
        <dd>
          Eight tools, plus <code>task</code> for a second agent. Hand out a
          subset with <code>-tools read,grep</code>.
        </dd>
      </div>
      <div>
        <dt>Closure</dt>
        <dd>Four permission modes. <kbd>⇧⇥</kbd> cycles them.</dd>
      </div>
      <div>
        <dt>Compartments</dt>
        <dd>
          Anthropic and Gemini. A third means implementing
          <code>llm.Provider</code>, and nothing else.
        </dd>
      </div>
      <div>
        <dt>Attachments</dt>
        <dd>MCP servers over stdio, declared in <code>mcp.json</code>.</dd>
      </div>
      <div>
        <dt>Overflow</dt>
        <dd>
          When the window fills, the conversation folds into a summary and
          carries on.
        </dd>
      </div>
      <div>
        <dt>Warranty</dt>
        <dd>MIT.</dd>
      </div>
    </dl>
  </div>
</section>

<style>
  .specs { padding-top: 76px; }
  .spec { margin: 0; border-top: 3px solid var(--ink); --step-ms: 45ms; }
  .spec > div {
    display: grid; grid-template-columns: 210px 1fr; gap: 20px;
    padding: 15px 4px; border-bottom: 2px dotted color-mix(in oklab, var(--ink) 30%, transparent);
  }
  .spec > div { transition: background .2s ease; }
  .spec > div:hover { background: color-mix(in oklab, var(--mustard) 14%, transparent); }

  .spec dt {
    font-family: var(--mono); font-size: 12.5px; font-weight: 700;
    letter-spacing: .1em; text-transform: uppercase; color: var(--rust);
    padding-top: 3px;
  }
  .spec dd { margin: 0; font-size: 16.5px; }
  @media (max-width: 640px) { .spec > div { grid-template-columns: 1fr; gap: 4px; } }
</style>
