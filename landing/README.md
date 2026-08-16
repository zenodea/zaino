# zaino landing

The landing page, as a SvelteKit site prerendered to static HTML.

```sh
npm install
npm run dev      # http://localhost:5173
npm run build    # static site in build/
npm run preview  # serve build/ as it will be served in production
```

`build/` is plain files — no server, no runtime.

## Layout

```
src/
  app.html                  document shell; sets the theme before first paint
  app.css                   colour tokens, base type, the classes sections share
  lib/
    content.js              the page's data: nav, installs, tools, commands, keys
    theme.svelte.js         auto / light / dark, remembered in localStorage
    components/             pieces used in more than one place
    sections/               one file per band of the page, top to bottom
  routes/
    +layout.svelte          header, footer, skip link
    +page.svelte            the sections, in order, plus <head> meta
```

Styles live with the markup they belong to. Only what is genuinely shared —
tokens, `.wrap`, `.band`, `.kick`, `h2.big`, `.sub`, `.body-copy`, `.notes` —
sits in `app.css`.
