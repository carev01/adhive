<script lang="ts">
	import '../../app.css';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { logout } from '$lib/api/auth';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';

	let mobileMenuOpen = false;
	let userEmail = 'User';

	onMount(() => {
		if (browser) {
			userEmail = localStorage.getItem('user_email') || 'User';
		}
	});

	async function handleLogout() {
		await logout();
		goto('/login');
	}
</script>

<div class="min-h-screen bg-slate-50 dark:bg-slate-900 transition-colors">
	<!-- Header -->
	<header class="bg-white dark:bg-slate-800 border-b border-slate-200 dark:border-slate-700">
		<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
			<div class="flex justify-between h-16">
				<div class="flex">
					<!-- Logo -->
					<a href="/entries" class="flex items-center gap-2">
						<div class="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center">
							<svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
							</svg>
						</div>
						<span class="text-xl font-bold text-slate-900 dark:text-white font-brand tracking-tight">Adhive</span>
					</a>
					
					<!-- Desktop Nav -->
					<nav class="hidden sm:ml-8 sm:flex sm:space-x-8">
						<a 
							href="/entries" 
							class="inline-flex items-center px-1 pt-1 text-sm font-medium {$page.url.pathname === '/entries' || $page.url.pathname.startsWith('/entries/') ? 'text-blue-600 dark:text-blue-400 border-b-2 border-blue-600 dark:border-blue-400' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'}"
						>
							Entries
						</a>
						<a 
							href="/tags" 
							class="inline-flex items-center px-1 pt-1 text-sm font-medium {$page.url.pathname === '/tags' ? 'text-blue-600 dark:text-blue-400 border-b-2 border-blue-600 dark:border-blue-400' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'}"
						>
							Tags
						</a>
					</nav>
				</div>

				<!-- User Menu -->
				<div class="hidden sm:flex sm:items-center">
					<div class="flex items-center gap-4">
						<ThemeToggle />
						<span class="text-sm text-slate-600 dark:text-slate-300">{userEmail}</span>
						<button
							on:click={handleLogout}
							class="text-sm text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200"
						>
							Logout
						</button>
					</div>
				</div>

				<!-- Mobile menu button -->
				<div class="flex items-center sm:hidden">
					<button
						on:click={() => mobileMenuOpen = !mobileMenuOpen}
						class="inline-flex items-center justify-center p-2 rounded-md text-slate-400 hover:text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-700"
					>
						<span class="sr-only">Open menu</span>
						{#if mobileMenuOpen}
							<svg class="block h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
							</svg>
						{:else}
							<svg class="block h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
							</svg>
						{/if}
					</button>
				</div>
			</div>
		</div>

		<!-- Mobile Menu -->
		{#if mobileMenuOpen}
			<div class="sm:hidden border-t border-slate-200 dark:border-slate-700">
				<div class="pt-2 pb-3 space-y-1 px-4">
					<a 
						href="/entries" 
						class="block pl-3 pr-4 py-2 text-base font-medium {$page.url.pathname === '/entries' || $page.url.pathname.startsWith('/entries/') ? 'text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-slate-700' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-700'}"
					>
						Entries
					</a>
					<a 
						href="/tags" 
						class="block pl-3 pr-4 py-2 text-base font-medium {$page.url.pathname === '/tags' ? 'text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-slate-700' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-700'}"
					>
						Tags
					</a>
					<div class="flex items-center pl-3 pr-4 py-2">
						<ThemeToggle />
					</div>
					<button
						on:click={handleLogout}
						class="block w-full text-left pl-3 pr-4 py-2 text-base font-medium text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-700"
					>
						Logout
					</button>
				</div>
			</div>
		{/if}
	</header>

	<!-- Main Content -->
	<main class="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
		<slot />
	</main>
</div>