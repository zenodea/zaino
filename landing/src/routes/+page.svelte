<script>
  import Stitch from '$lib/components/Stitch.svelte';
  import Hero from '$lib/sections/Hero.svelte';
  import Packing from '$lib/sections/Packing.svelte';
  import Repl from '$lib/sections/Repl.svelte';
  import Tools from '$lib/sections/Tools.svelte';
  import Pockets from '$lib/sections/Pockets.svelte';
  import Journey from '$lib/sections/Journey.svelte';
  import Commands from '$lib/sections/Commands.svelte';
  import Cta from '$lib/sections/Cta.svelte';

  const title = 'Zaino: an agent harness you can carry';
  const description =
    'Zaino is an agent harness in Go. Five hand-rolled provider clients, eight tools, subagents, MCP and branching sessions, in one binary. Italian for backpack.';
</script>

<svelte:head>
  <title>{title}</title>
  <meta name="description" content={description} />
  <meta property="og:title" content={title} />
  <meta
    property="og:description"
    content="Five hand-rolled provider clients, eight tools, subagents and MCP, in one binary. Italian for backpack."
  />
</svelte:head>

<Hero />
<Stitch />
<Packing />
<Stitch />
<Repl />
<Tools />
<Pockets />
<Stitch />
<Journey />
<Commands />
<Cta />
