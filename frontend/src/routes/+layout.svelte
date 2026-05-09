<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import Toast from '../components/Toast.svelte';
	import { subscribeState, type WsState } from '$lib/ws';

	interface Props {
		children: import('svelte').Snippet;
	}

	let { children }: Props = $props();
	let user = $state<{ id: number; email: string } | null>(null);
	let loading = $state(true);
	let loginError = $state('');
	let mobileNavOpen = $state(false);
	let wsState = $state<WsState>('disconnected');
	let unsubscribeWs: (() => void) | null = null;
	let hamburgerBtn: HTMLButtonElement | null = $state(null);
	let mobileNav: HTMLElement | null = $state(null);

	$effect(() => {
		if (!mobileNavOpen) return;
		// Move focus into the drawer so keyboard / screen-reader users can
		// navigate it without first tabbing through the rest of the page.
		const target = mobileNav?.querySelector<HTMLElement>('a, button');
		target?.focus();
	});

	function handleNavKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && mobileNavOpen) {
			closeMobileNav();
		}
	}

	onMount(async () => {
		const session = await api.session();
		user = session.authenticated ? (session.user ?? null) : null;
		loading = false;
		unsubscribeWs = subscribeState((s) => (wsState = s));
	});

	onDestroy(() => {
		unsubscribeWs?.();
	});

	async function logout() {
		await api.logout();
		user = null;
	}

	function closeMobileNav() {
		mobileNavOpen = false;
		hamburgerBtn?.focus();
	}

	const navItems = [
		{ href: '/', label: 'Overview', icon: '⊞' },
		{ href: '/security', label: 'Security', icon: '⊘' },
		{ href: '/cron', label: 'Cron', icon: '◷' },
		{ href: '/settings', label: 'Settings', icon: '⚙' }
	];

	const wsLabels: Record<WsState, string> = {
		connecting: 'Connecting…',
		connected: 'Live',
		disconnected: 'Disconnected'
	};
	const wsDotClass: Record<WsState, string> = {
		connecting: 'bg-warning animate-pulse',
		connected: 'bg-success',
		disconnected: 'bg-danger'
	};
</script>

<svelte:window onkeydown={handleNavKey} />

{#if loading}
	<div class="h-screen flex items-center justify-center">
		<div class="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin"></div>
	</div>
{:else if !user}
	<!-- Login form -->
	<div class="h-screen flex items-center justify-center px-4">
		<form
			class="bg-bg-card border border-border rounded-xl p-8 w-full max-w-sm"
			onsubmit={async (e) => {
				e.preventDefault();
				loginError = '';
				const form = e.currentTarget;
				const data = new FormData(form);
				try {
					await api.login(data.get('email') as string, data.get('password') as string);
					const session = await api.session();
					user = session.authenticated ? (session.user ?? null) : null;
					loginError = '';
				} catch (err) {
					loginError = err instanceof Error ? err.message : 'Sign in failed';
				}
			}}
		>
			<h1 class="text-xl font-semibold mb-6 text-center">Dashboard</h1>
			<label for="login-email" class="sr-only">Email</label>
			<input
				id="login-email"
				name="email"
				type="email"
				placeholder="Email"
				autocomplete="email"
				required
				aria-invalid={loginError ? 'true' : undefined}
				class="w-full bg-bg border border-border rounded-lg px-4 py-2.5 mb-3 text-sm focus:outline-none focus:border-accent transition-colors"
			/>
			<label for="login-password" class="sr-only">Password</label>
			<input
				id="login-password"
				name="password"
				type="password"
				placeholder="Password"
				autocomplete="current-password"
				required
				aria-invalid={loginError ? 'true' : undefined}
				aria-describedby={loginError ? 'login-error' : undefined}
				class="w-full bg-bg border border-border rounded-lg px-4 py-2.5 mb-2 text-sm focus:outline-none focus:border-accent transition-colors"
			/>
			{#if loginError}
				<p id="login-error" role="alert" class="mb-4 text-sm text-danger">{loginError}</p>
			{:else}
				<div class="mb-4"></div>
			{/if}
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
		<!-- Sidebar (desktop static, mobile drawer) -->
		{#if mobileNavOpen}
			<button
				type="button"
				class="fixed inset-0 z-30 bg-black/50 md:hidden"
				aria-label="Close navigation"
				onclick={closeMobileNav}
			></button>
		{/if}
		<nav
			bind:this={mobileNav}
			id="primary-nav"
			aria-label="Primary"
			class="fixed inset-y-0 left-0 z-40 w-56 bg-bg-card border-r border-border flex flex-col shrink-0 transform transition-transform duration-200
				md:static md:translate-x-0
				{mobileNavOpen ? 'translate-x-0' : '-translate-x-full'}"
		>
			<div class="px-5 py-5 border-b border-border flex items-center justify-between">
				<h1 class="text-sm font-semibold tracking-wide text-accent">DASHBOARD</h1>
				<button
					type="button"
					class="md:hidden text-text-dim hover:text-text cursor-pointer"
					aria-label="Close navigation"
					onclick={closeMobileNav}
				>
					✕
				</button>
			</div>

			<div class="flex-1 py-3">
				{#each navItems as item}
					<a
						href={item.href}
						onclick={closeMobileNav}
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
				<div class="text-xs text-text-dim mb-2 break-all">{user.email}</div>
				<button
					onclick={logout}
					class="text-xs text-text-dim hover:text-danger transition-colors cursor-pointer"
				>
					Sign out
				</button>
			</div>
		</nav>

		<!-- Main content -->
		<main class="flex-1 overflow-y-auto">
			<header class="md:hidden sticky top-0 z-20 flex items-center justify-between gap-3 border-b border-border bg-bg-card px-4 py-3">
				<button
					bind:this={hamburgerBtn}
					type="button"
					class="text-text hover:text-accent cursor-pointer"
					aria-label="Open navigation"
					aria-expanded={mobileNavOpen}
					aria-controls="primary-nav"
					onclick={() => (mobileNavOpen = true)}
				>
					<svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
						<path d="M3 6h18M3 12h18M3 18h18" />
					</svg>
				</button>
				<h1 class="text-sm font-semibold tracking-wide text-accent">DASHBOARD</h1>
				<span
					class="flex items-center gap-1.5 text-xs text-text-dim"
					title={wsLabels[wsState]}
				>
					<span class="h-2 w-2 rounded-full {wsDotClass[wsState]}" aria-hidden="true"></span>
					<span class="sr-only">{wsLabels[wsState]}</span>
				</span>
			</header>
			<div
				class="absolute right-4 top-4 z-10 hidden items-center gap-2 rounded-full border border-border bg-bg-card px-3 py-1 text-xs text-text-dim md:flex"
				title={wsLabels[wsState]}
				aria-live="polite"
			>
				<span class="h-2 w-2 rounded-full {wsDotClass[wsState]}" aria-hidden="true"></span>
				{wsLabels[wsState]}
			</div>
			<div class="p-4 md:p-6">
				{@render children()}
			</div>
		</main>
	</div>
{/if}

<Toast />
