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
  { href: '#hooks', label: 'hooks' },
  { href: '#vim', label: 'vim' },
  { href: '#journey', label: 'journey' },
  { href: '#drive', label: 'commands' }
];

export const packParts = [
  { id: 'bedroll', part: 'The bedroll', feat: 'five hand-rolled clients' },
  { id: 'main', part: 'Main compartment', feat: 'the turn loop' },
  { id: 'pocket', part: 'Front pocket', feat: 'twelve tools' },
  { id: 'lid', part: 'The lid', feat: 'one binary, no runtime' },
  { id: 'buckle', part: 'The buckle', feat: 'asks before it acts' },
  { id: 'side', part: 'Side pocket', feat: 'MCP, over stdio' },
  { id: 'base', part: 'Reinforced base', feat: 'tested throughout' }
];

export const commands = [
  [
    ['/model', 'show or change the model'],
    ['/provider', 'switch provider'],
    ['/effort', 'how hard it thinks before answering'],
    ['/profile', 'a named bundle of settings'],
    ['/permission', 'when tools stop to ask'],
    ['/limit', 'a ceiling on the context'],
    ['/spend', 'a ceiling on the cost'],
    ['/usage', 'tokens and dollars so far']
  ],
  [
    ['/rewind', 'back to an earlier prompt'],
    ['/journey', 'the whole tree, every branch'],
    ['/sessions', 'pick up an earlier conversation'],
    ['/files', 'what write and edit changed here'],
    ['/todo', 'the plan, and how far it is'],
    ['/memory', 'what it has noted about this project'],
    ['/jobs', 'what is running in the background'],
    ['/hooks', 'the scripts your config runs']
  ]
];

export const keyHints = [
  { keys: ['⏎'], label: 'send' },
  { keys: ['⌥⏎'], label: 'newline' },
  { keys: ['⌃j', '⌃k'], label: 'walk the transcript' },
  { keys: ['↑', '↓'], label: 'earlier prompts' },
  { keys: ['⇧⇥'], label: 'permission mode' },
  { keys: ['esc'], label: 'stop the turn' },
  { keys: ['⌃c'], label: 'stop, then quit' },
  { keys: ['PgUp', 'PgDn'], label: 'scroll' }
];
