import { browser } from '$app/environment';

const modes = ['auto', 'light', 'dark'];
const saved = browser ? localStorage.getItem('zaino-theme') : null;

export const theme = $state({ mode: modes.includes(saved) ? saved : 'auto' });

export function cycleTheme() {
  theme.mode = modes[(modes.indexOf(theme.mode) + 1) % modes.length];
  document.documentElement.dataset.theme = theme.mode;
  localStorage.setItem('zaino-theme', theme.mode);
}
