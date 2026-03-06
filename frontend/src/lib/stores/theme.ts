import { writable } from 'svelte/store';
import { browser } from '$app/environment';

type Theme = 'light' | 'dark' | 'system';

function getSystemDark(): boolean {
  if (!browser) return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function applyTheme(theme: Theme) {
  if (!browser) return;
  
  const root = document.documentElement;
  const systemDark = getSystemDark();
  
  if (theme === 'dark' || (theme === 'system' && systemDark)) {
    root.classList.add('dark');
  } else {
    root.classList.remove('dark');
  }
}

function createThemeStore() {
  const defaultTheme: Theme = 'system';
  
  // Get initial theme from localStorage or default
  const stored = browser ? localStorage.getItem('theme') as Theme : null;
  const initial = stored || defaultTheme;
  
  const { subscribe, set, update } = writable<Theme>(initial);
  
  // Apply theme on init
  if (browser) {
    applyTheme(initial);
  }
  
  return {
    subscribe,
    set: (value: Theme) => {
      if (browser) {
        localStorage.setItem('theme', value);
        applyTheme(value);
      }
      set(value);
    },
    toggle: () => {
      update(current => {
        const newTheme = current === 'light' ? 'dark' : 'light';
        if (browser) {
          localStorage.setItem('theme', newTheme);
          applyTheme(newTheme);
        }
        return newTheme;
      });
    },
    getCurrentTheme: (): Theme => {
      let current: Theme = 'system';
      subscribe(v => current = v)();
      return current;
    }
  };
}

export const theme = createThemeStore();

// Listen for system theme changes when in system mode
if (browser) {
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    const stored = localStorage.getItem('theme') as Theme || 'system';
    if (stored === 'system') {
      applyTheme('system');
    }
  });
}
