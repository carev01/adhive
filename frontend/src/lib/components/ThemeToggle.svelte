<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';

  let isDark = false;
  let mounted = false;

  onMount(() => {
    mounted = true;
    updateState();
  });

  function updateState() {
    if (!browser) return;
    
    const stored = localStorage.getItem('theme') as 'light' | 'dark' | 'system' || 'system';
    const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    isDark = stored === 'dark' || (stored === 'system' && systemDark);
    
    // Apply to DOM
    if (isDark) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }

  function toggle() {
    if (!browser) return;
    
    const stored = localStorage.getItem('theme') as 'light' | 'dark' | 'system' || 'system';
    let newTheme: 'light' | 'dark';
    
    if (stored === 'system') {
      // If system, toggle based on current actual state
      newTheme = isDark ? 'light' : 'dark';
    } else {
      // Toggle from current
      newTheme = stored === 'dark' ? 'light' : 'dark';
    }
    
    localStorage.setItem('theme', newTheme);
    updateState();
  }

  // Listen for system theme changes
  onMount(() => {
    if (!browser) return;
    
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => {
      const stored = localStorage.getItem('theme') as 'light' | 'dark' | 'system' || 'system';
      if (stored === 'system') {
        updateState();
      }
    };
    
    mediaQuery.addEventListener('change', handler);
    return () => mediaQuery.removeEventListener('change', handler);
  });
</script>

<button
  on:click={toggle}
  class="relative inline-flex h-9 w-14 items-center rounded-full border-2 border-slate-300 dark:border-slate-600 transition-colors hover:border-slate-400 dark:hover:border-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-slate-900"
  aria-label={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
  title={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
>
  <span
    class="inline-flex h-7 w-7 items-center justify-center rounded-full bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-300 transition-transform duration-200 ease-spring"
    class:translate-x-5={isDark}
    class:translate-x-0={!isDark}
  >
    {#if isDark}
      <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
        <path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z" />
      </svg>
    {:else}
      <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
        <path fill-rule="evenodd" d="M10 2a1 1 0 011 1v1a1 1 0 11-2 0V3a1 1 0 011-1zm4 8a4 4 0 11-8 0 4 4 0 018 0zm-.464 4.95l.707.707a1 1 0 001.414-1.414l-.707-.707a1 1 0 00-1.414 1.414zm2.12-10.607a1 1 0 010 1.414l-.706.707a1 1 0 11-1.414-1.414l.707-.707a1 1 0 011.414 0zM17 11a1 1 0 100-2h-1a1 1 0 100 2h1zm-7 4a1 1 0 011 1v1a1 1 0 11-2 0v-1a1 1 0 011-1zM5.05 6.464A1 1 0 106.465 5.05l-.708-.707a1 1 0 00-1.414 1.414l.707.707zm1.414 8.486l-.707.707a1 1 0 01-1.414-1.414l.707-.707a1 1 0 011.414 1.414zM4 11a1 1 0 100-2H3a1 1 0 000 2h1z" clip-rule="evenodd" />
      </svg>
    {/if}
  </span>
</button>

<style>
  .ease-spring {
    transition-timing-function: cubic-bezier(0.34, 1.56, 0.64, 1);
  }
</style>
