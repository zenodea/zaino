<script>
  import { reveal } from '$lib/reveal.js';
</script>

<section class="band care" id="care">
  <div class="wrap care-grid" data-stagger use:reveal>
    <div>
      <p class="kick">Sewn into the lining</p>
      <h2 class="big">Two rules keep<br /><code>edit</code> honest.</h2>
      <p class="body-copy">
        They are about correctness rather than permission, which is a separate
        question.
      </p>
      <p class="body-copy dim">
        If a file changes on disk between an edit being worked out and being
        allowed, the write is refused rather than clobbering whoever else was in
        there.
      </p>
    </div>

    <div class="label lift">
      <p class="label-top">CARE INSTRUCTIONS</p>
      <ol>
        <li>
          <b>Read it first.</b>
          <span>
            A file must have been read this session before it can be edited or
            overwritten. Nothing is ever written blind.
          </span>
        </li>
        <li>
          <b>Say what you expect to find.</b>
          <span>
            An edit replaces an exact stretch of text that must appear exactly
            once. Near misses are forgiven only where the difference can never
            matter — curly quotes, dashes, trailing space — and the result says
            so when it happened.
          </span>
        </li>
        <li>
          <b>All of them, or none.</b>
          <span>
            <code>edit</code> takes an <code>edits</code> array to change several
            places in one call. They apply in order, each seeing the result of
            the last — and if any one fails to match, none are written.
          </span>
        </li>
      </ol>
      <p class="label-bot">DO NOT WRITE BLIND</p>
    </div>
  </div>
</section>

<style>
  .care { background: var(--paper); border-top: 3px solid var(--ink); border-bottom: 3px solid var(--ink); }
  .care-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 52px; align-items: center; }
  @media (max-width: 900px) { .care-grid { grid-template-columns: 1fr; gap: 34px; } }

  .label {
    background: var(--canvas);
    border: 3px solid var(--ink);
    box-shadow: 6px 6px 0 var(--shadow);
    border-radius: 4px;
    padding: 20px 24px 16px;
    transform: rotate(-1deg);
    font-family: var(--mono);
  }
  .label:hover { box-shadow: 9px 9px 0 var(--shadow); }
  .label-top, .label-bot {
    text-align: center; font-size: 10.5px; font-weight: 700;
    letter-spacing: .16em; color: var(--faint); margin: 0;
  }
  .label-top { border-bottom: 2px dashed var(--ink); padding-bottom: 10px; margin-bottom: 14px; }
  .label-bot { border-top: 2px dashed var(--ink); padding-top: 10px; margin-top: 14px; }

  .label ol { margin: 0; padding-left: 20px; }
  .label li { margin-bottom: 14px; font-size: 13.5px; line-height: 1.7; }
  .label li:last-child { margin-bottom: 0; }
  .label li b {
    display: block; font-family: var(--disp); font-style: italic;
    font-size: 20px; font-weight: 800; color: var(--rust); margin-bottom: 3px;
  }
  .label li span { color: var(--ink-2); }
  .label li::marker { color: var(--rust); font-weight: 700; }
</style>
