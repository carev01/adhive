<script lang="ts">
	import { goto } from '$app/navigation';
	import { register } from '$lib/api/auth';

	let email = '';
	let password = '';
	let passwordConfirm = '';
	let error = '';
	let loading = false;

	async function handleSubmit() {
		error = '';
		loading = true;

		if (!email || !password || !passwordConfirm) {
			error = 'Please fill in all fields';
			loading = false;
			return;
		}

		if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
			error = 'Please enter a valid email address';
			loading = false;
			return;
		}

		if (password.length < 8) {
			error = 'Password must be at least 8 characters';
			loading = false;
			return;
		}

		if (password !== passwordConfirm) {
			error = 'Passwords do not match';
			loading = false;
			return;
		}

		const result = await register({ email, password, passwordConfirm });

		if (result.error) {
			error = result.error;
		} else {
			goto('/entries');
		}
		loading = false;
	}
</script>

<svelte:head>
	<title>Create Account - Adhive</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center bg-slate-50 dark:bg-slate-900 py-12 px-4">
	<div class="max-w-md w-full">
		<div class="text-center mb-8">
			<a href="/" class="inline-flex items-center gap-2">
				<div class="w-10 h-10 rounded-lg bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center">
					<svg class="w-6 h-6 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
					</svg>
				</div>
				<span class="text-2xl font-bold text-slate-900 dark:text-white font-brand tracking-tight">Adhive</span>
			</a>
			<p class="text-slate-600 dark:text-slate-400 mt-3">Create your account</p>
		</div>

		<div class="bg-white dark:bg-slate-800 rounded-xl shadow-sm border border-slate-200 dark:border-slate-700 p-8">
			<form on:submit|preventDefault={handleSubmit} class="space-y-6">
				{#if error}
					<div class="bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 px-4 py-3 rounded-lg text-sm">
						{error}
					</div>
				{/if}

				<div>
					<label for="email" class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">Email</label>
					<input
						type="email"
						id="email"
						bind:value={email}
						placeholder="you@example.com"
						class="w-full px-4 py-3 rounded-lg border border-slate-300 dark:border-slate-600 focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none transition bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
					/>
				</div>

				<div>
					<label for="password" class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">Password</label>
					<input
						type="password"
						id="password"
						bind:value={password}
						placeholder="Min. 8 characters"
						class="w-full px-4 py-3 rounded-lg border border-slate-300 dark:border-slate-600 focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none transition bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
					/>
					<p class="mt-1 text-xs text-slate-500 dark:text-slate-400">Must include 3 of: uppercase, lowercase, number, special character</p>
				</div>

				<div>
					<label for="passwordConfirm" class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">Confirm Password</label>
					<input
						type="password"
						id="passwordConfirm"
						bind:value={passwordConfirm}
						placeholder="••••••••"
						class="w-full px-4 py-3 rounded-lg border border-slate-300 dark:border-slate-600 focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none transition bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
					/>
				</div>

				<button
					type="submit"
					disabled={loading}
					class="w-full py-3 px-4 bg-blue-600 text-white font-medium rounded-lg hover:bg-blue-700 focus:ring-4 focus:ring-blue-200 dark:focus:ring-blue-800 transition disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{loading ? 'Creating account...' : 'Create Account'}
				</button>
			</form>

			<p class="mt-6 text-center text-sm text-slate-600 dark:text-slate-400">
				Already have an account?
				<a href="/login" class="text-blue-600 dark:text-blue-400 hover:underline font-medium">Sign in</a>
			</p>
		</div>
	</div>
</div>