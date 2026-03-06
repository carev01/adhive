<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';

	onMount(() => {
		if (browser) {
			initTheme();
		}
	});

	function initTheme() {
		const stored = localStorage.getItem('theme') as 'light' | 'dark' | 'system' || 'system';
		const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
		const isDark = stored === 'dark' || (stored === 'system' && systemDark);
		
		if (isDark) {
			document.documentElement.classList.add('dark');
		} else {
			document.documentElement.classList.remove('dark');
		}
		
		// Listen for system theme changes
		window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
			const current = localStorage.getItem('theme') as 'light' | 'dark' | 'system' || 'system';
			if (current === 'system') {
				if (e.matches) {
					document.documentElement.classList.add('dark');
				} else {
					document.documentElement.classList.remove('dark');
				}
			}
		});
	}
</script>

<slot />
