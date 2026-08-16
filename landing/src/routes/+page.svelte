<script>
  import Stitch from '$lib/components/Stitch.svelte';
  import Hero from '$lib/sections/Hero.svelte';
  import Packing from '$lib/sections/Packing.svelte';
  import Specs from '$lib/sections/Specs.svelte';
  import Providers from '$lib/sections/Providers.svelte';
  import Tools from '$lib/sections/Tools.svelte';
  import Pockets from '$lib/sections/Pockets.svelte';
  import Care from '$lib/sections/Care.svelte';
  import Compaction from '$lib/sections/Compaction.svelte';
  import Commands from '$lib/sections/Commands.svelte';
  import Cta from '$lib/sections/Cta.svelte';

  const title = 'Zaino — an agent harness you can carry';
  const description =
    'Zaino is an agent harness in Go. Two hand-rolled provider clients, eight tools, subagents, MCP and compaction — in one binary. Italian for backpack.';
</script>

<svelte:head>
  <title>{title}</title>
  <meta name="description" content={description} />
  <meta property="og:title" content={title} />
  <meta
    property="og:description"
    content="Two hand-rolled provider clients, eight tools, subagents and MCP, in one binary. Italian for backpack."
  />
</svelte:head>

<Hero />
<Stitch />
<Packing />
<Specs />
<Stitch />
<Providers />
<Tools />
<Pockets />
<Stitch />
<Care />
<Compaction />
<Commands />
<Cta />
