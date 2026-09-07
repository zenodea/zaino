<script>
  import Seam from '$lib/components/Seam.svelte';
  import Hero from '$lib/sections/Hero.svelte';
  import Packing from '$lib/sections/Packing.svelte';
  import Repl from '$lib/sections/Repl.svelte';
  import Hooks from '$lib/sections/Hooks.svelte';
  import Vim from '$lib/sections/Vim.svelte';
  import Journey from '$lib/sections/Journey.svelte';
  import Commands from '$lib/sections/Commands.svelte';
  import Cta from '$lib/sections/Cta.svelte';

  const title = 'Zaino: an agent harness you can carry';
  const description =
    'Zaino is a coding agent in one Go binary. Five hand-rolled provider clients, twelve tools, subagents, MCP, hooks, and sessions kept as a tree with the files restored to match. Italian for backpack.';
</script>

<svelte:head>
  <title>{title}</title>
  <meta name="description" content={description} />
  <meta property="og:title" content={title} />
  <meta
    property="og:description"
    content="A coding agent in one Go binary: five hand-rolled provider clients, twelve tools, subagents, MCP and branching sessions. Italian for backpack."
  />
</svelte:head>

<Hero />
<Seam kind="stitch" />
<Packing />
<Seam kind="zip" />
<Repl />
<Seam kind="tape" />
<Hooks />
<Seam kind="stitch" />
<Vim />
<Seam kind="buckle" />
<Journey />
<Seam kind="zip" />
<Commands />
<Seam kind="tape" />
<Cta />
