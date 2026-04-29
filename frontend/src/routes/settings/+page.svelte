<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

	let hiddenCronCount = $state(0);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let message = $state('');

	onMount(loadSettings);

	async function loadSettings() {
		loading = true;
		error = '';
		try {
			const result = await api.hiddenCronJobCount();
			hiddenCronCount = result.count;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load settings';
		} finally {
			loading = false;
		}
	}

	async function resetHiddenCronJobs() {
		saving = true;
		error = '';
		message = '';
		try {
			await api.resetHiddenCronJobs();
			hiddenCronCount = 0;
			message = 'Hidden cron jobs reset';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to reset hidden cron jobs';
		} finally {
			saving = false;
		}
	}
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-lg font-semibold">Settings</h2>
		<p class="text-sm text-text-dim">Dashboard preferences</p>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<div class="h-6 w-6 animate-spin rounded-full border-2 border-accent border-t-transparent"></div>
		</div>
	{:else}
		<section class="rounded-lg border border-border bg-bg-card p-5">
			<div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
				<div>
					<h3 class="text-sm font-semibold">Hidden cron jobs</h3>
					<p class="mt-1 text-sm text-text-dim">{hiddenCronCount} hidden job{hiddenCronCount === 1 ? '' : 's'}</p>
				</div>
				<button
					class="rounded bg-accent/10 px-4 py-2 text-sm text-accent transition hover:bg-accent/20 disabled:cursor-not-allowed disabled:opacity-50"
					onclick={resetHiddenCronJobs}
					disabled={saving || hiddenCronCount === 0}
				>
					{saving ? 'Resetting...' : 'Reset hidden cron'}
				</button>
			</div>
			{#if error}
				<p class="mt-4 rounded border border-danger/30 bg-danger/10 p-3 text-sm text-danger">{error}</p>
			{/if}
			{#if message}
				<p class="mt-4 rounded border border-success/30 bg-success/10 p-3 text-sm text-success">{message}</p>
			{/if}
		</section>
	{/if}
</div>
