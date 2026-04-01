<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';

	interface Props {
		children: import('svelte').Snippet;
	}

	let { children }: Props = $props();
	let user = $state<{ id: number; email: string } | null>(null);
	let loading = $state(true);

	onMount(async () => {
		try {
			user = await api.me();
		} catch {
			user = null;
		}
		loading = false;
	});

	async function logout() {
		await api.logout();
		user = null;
	}

	const navItems = [
		{ href: '/', label: 'Overview', icon: '⊞' },
		{ href: '/security', label: 'Security', icon: '⊘' }
	];
</script>

{#if loading}
	<div class="h-screen flex items-center justify-center">
		<div class="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin"></div>
	</div>
{:else if !user}
	<!-- Login form -->
	<div class="h-screen flex items-center justify-center">
		<form
			class="bg-bg-card border border-border rounded-xl p-8 w-full max-w-sm"
			onsubmit={async (e) => {
				e.preventDefault();
				const form = e.currentTarget;
				const data = new FormData(form);
				try {
					await api.login(data.get('email') as string, data.get('password') as string);
					user = await api.me();
				} catch (err) {
					// show error
				}
			}}
		>
			<h1 class="text-xl font-semibold mb-6 text-center">Dashboard</h1>
			<input
				name="email"
				type="email"
				placeholder="Email"
				required
				class="w-full bg-bg border border-border rounded-lg px-4 py-2.5 mb-3 text-sm focus:outline-none focus:border-accent transition-colors"
			/>
			<input
				name="password"
				type="password"
				placeholder="Password"
				required
				class="w-full bg-bg border border-border rounded-lg px-4 py-2.5 mb-4 text-sm focus:outline-none focus:border-accent transition-colors"
			/>
			<button
				type="submit"
				class="w-full bg-accent text-bg font-medium rounded-lg py-2.5 text-sm hover:opacity-90 transition-opacity cursor-pointer"
			>
				Sign in
			</button>
		</form>
	</div>
{:else}
	<!-- App shell -->
	<div class="flex h-screen">
		<!-- Sidebar -->
		<nav class="w-56 bg-bg-card border-r border-border flex flex-col shrink-0">
			<div class="px-5 py-5 border-b border-border">
				<h1 class="text-sm font-semibold tracking-wide text-accent">DASHBOARD</h1>
			</div>

			<div class="flex-1 py-3">
				{#each navItems as item}
					<a
						href={item.href}
						class="flex items-center gap-3 px-5 py-2.5 text-sm transition-colors
							{page.url.pathname === item.href
								? 'text-accent bg-accent-dim'
								: 'text-text-dim hover:text-text hover:bg-bg-hover'}"
					>
						<span class="text-base">{item.icon}</span>
						{item.label}
					</a>
				{/each}
			</div>

			<div class="px-5 py-4 border-t border-border">
				<div class="text-xs text-text-dim mb-2">{user.email}</div>
				<button
					onclick={logout}
					class="text-xs text-text-dim hover:text-danger transition-colors cursor-pointer"
				>
					Sign out
				</button>
			</div>
		</nav>

		<!-- Main content -->
		<main class="flex-1 overflow-y-auto p-6">
			{@render children()}
		</main>
	</div>
{/if}
