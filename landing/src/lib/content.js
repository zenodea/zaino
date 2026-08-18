export const repo = 'https://github.com/zenodea/Zaino';

export const installCommand =
  'curl -fsSL https://raw.githubusercontent.com/zenodea/Zaino/main/install.sh | sh';

export const installs = [
  { id: 'sh', label: 'install.sh', cmd: installCommand },
  { id: 'go', label: 'go install', cmd: 'go install github.com/zenodea/zaino/cmd/zaino@latest' },
  { id: 'src', label: 'source', cmd: 'git clone https://github.com/zenodea/Zaino && ./install.sh' }
];

export const nav = [
  { href: '#pack', label: 'the pack' },
  { href: '#reach', label: 'tools' },
  { href: '#pockets', label: 'pockets' },
  { href: '#journey', label: 'journey' },
  { href: '#drive', label: 'commands' }
];

export const packParts = [
  { part: 'The lid', feat: 'five hand-rolled clients' },
  { part: 'Main compartment', feat: 'the turn loop' },
  { part: 'Front pocket', feat: 'eight tools' },
  { part: 'Haul loop', feat: 'one binary, no runtime' },
  { part: 'The buckle', feat: 'asks before it acts' },
  { part: 'Side pocket', feat: 'MCP, over stdio' },
  { part: 'Reinforced base', feat: 'tested throughout' }
];

export const tools = ['read', 'write', 'edit', 'bash', 'grep', 'find', 'ls', 'fetch'];

export const permissionModes = [
  {
    name: 'manual',
    desc: 'Ask before writing, running or fetching.',
    stamp: 'default'
  },
  {
    name: 'accept-edits',
    desc: 'Edits go through. Still asks before running or fetching.'
  },
  {
    name: 'plan',
    desc: 'Read only. Nothing written or run, but pages can still be read.'
  },
  {
    name: 'bypass',
    desc: 'Everything goes through unasked.',
    stamp: 'on you',
    warn: true,
    open: true
  }
];

export const commands = [
  [
    ['/clear', 'forget the conversation'],
    ['/model', 'show or change the model'],
    ['/provider', 'switch provider'],
    ['/effort', 'show or set output effort'],
    ['/thinking', "show or hide the model's reasoning"],
    ['/system', 'show, set, or drop the system prompt'],
    ['/profile', 'switch to a named bundle of settings'],
    ['/config', 'what the config files came to'],
    ['/compact', 'fold the conversation into a summary'],
    ['/limit', 'stop when the context passes a ceiling']
  ],
  [
    ['/rewind', 'take it up again from an earlier turn'],
    ['/journey', 'the tree of turns, every branch included'],
    ['/permission', 'when tools stop to ask'],
    ['/tools', 'list the tools the model has'],
    ['/usage', 'token usage for this session'],
    ['/sessions', 'pick up an earlier conversation'],
    ['/vim', 'modal editing in the composer'],
    ['/bro', 'say the last answer again, simply'],
    ['/help', 'list the commands'],
    ['/quit', 'leave zaino']
  ]
];

export const keyHints = [
  { keys: ['⏎'], label: 'send' },
  { keys: ['⌥⏎'], label: 'newline' },
  { keys: ['⌃j', '⌃k'], label: 'walk the chat' },
  { keys: ['↑', '↓'], label: 'earlier prompts' },
  { keys: ['⇧⇥'], label: 'permission mode' },
  { keys: ['esc'], label: 'stop the turn' },
  { keys: ['⌃c'], label: 'stop, then quit' },
  { keys: ['PgUp', 'PgDn'], label: 'scroll' }
];
