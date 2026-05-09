<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Fail2BanStatus, type BanEvent, type LogEntry } from '$lib/api';
	import { toastError } from '$lib/stores/toast.svelte';

	let f2bStatus = $state<Fail2BanStatus | null>(null);
	let banEvents = $state<BanEvent[]>([]);
	let logs = $state<LogEntry[]>([]);
	let logUnit = $state('');
	let logPriority = $state(-1);
	let loading = $state(true);

	onMount(async () => {
		await refresh();
		loading = false;
	});

	async function refresh() {
		const [status, bans, logEntries] = await Promise.all([
			api.fail2ban().catch((err) => {
				toastError(err, 'Failed to load fail2ban status');
				return null;
			}),
			api.fail2banBans().catch((err) => {
				toastError(err, 'Failed to load ban events');
				return [] as BanEvent[];
			}),
			api.logs(logUnit, logPriority).catch((err) => {
				toastError(err, 'Failed to load logs');
				return [] as LogEntry[];
			})
		]);
		f2bStatus = status;
		banEvents = bans;
		logs = logEntries;
	}

	async function filterLogs() {
		try {
			logs = await api.logs(logUnit, logPriority);
		} catch (err) {
			logs = [];
			toastError(err, 'Failed to filter logs');
		}
	}

	function formatTimestamp(ts: string): string {
		if (!ts) return '';
		// journalctl timestamps are microseconds
		const ms = parseInt(ts) / 1000;
		if (isNaN(ms)) return ts;
		return new Date(ms).toLocaleString();
	}

	const priorityColors: Record<string, string> = {
		emerg: 'text-danger font-bold',
		alert: 'text-danger font-bold',
		crit: 'text-danger',
		err: 'text-danger',
		warning: 'text-warning',
		notice: 'text-info',
		info: 'text-text-dim',
		debug: 'text-text-dim'
	};
</script>

<div class="space-y-6">
	<h2 class="text-lg font-semibold">Security & Alerts</h2>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<div class="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin"></div>
		</div>
	{:else}
		<!-- Fail2ban summary -->
		{#if f2bStatus}
			<div class="grid grid-cols-2 md:grid-cols-3 gap-4">
				<div class="bg-bg-card border border-border rounded-xl p-5">
					<div class="text-2xl font-semibold text-accent">{f2bStatus.totalJails}</div>
					<div class="text-sm text-text-dim mt-1">Active Jails</div>
				</div>
				<div class="bg-bg-card border border-border rounded-xl p-5">
					<div class="text-2xl font-semibold text-danger">{f2bStatus.totalBans}</div>
					<div class="text-sm text-text-dim mt-1">Currently Banned</div>
				</div>
				<div class="bg-bg-card border border-border rounded-xl p-5">
					<div class="text-2xl font-semibold text-warning">{banEvents.length}</div>
					<div class="text-sm text-text-dim mt-1">Recent Events</div>
				</div>
			</div>

			<!-- Jail details -->
			<div class="bg-bg-card border border-border rounded-xl p-4">
				<h3 class="text-sm font-medium text-text-dim mb-3">Fail2Ban Jails</h3>
				<div class="space-y-3">
					{#each f2bStatus.jails as jail}
						<div class="border border-border/50 rounded-lg p-3">
							<div class="flex items-center justify-between mb-2">
								<span class="font-mono text-sm">{jail.name}</span>
								<div class="flex gap-4 text-xs text-text-dim">
									<span>Banned: <span class="text-danger">{jail.banCount}</span></span>
									<span>Total bans: {jail.totalBans}</span>
									<span>Failed: {jail.totalFails}</span>
								</div>
							</div>
							{#if jail.bannedIPs && jail.bannedIPs.length > 0}
								<div class="flex flex-wrap gap-2 mt-2">
									{#each jail.bannedIPs as ip}
										<span class="bg-danger/15 text-danger text-xs font-mono px-2 py-0.5 rounded">
											{ip}
										</span>
									{/each}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{:else}
			<div class="bg-bg-card border border-border rounded-xl p-5 text-text-dim text-sm">
				Fail2ban data unavailable. Make sure fail2ban is running and accessible.
			</div>
		{/if}

		<!-- Recent ban events -->
		{#if banEvents.length > 0}
			<div class="bg-bg-card border border-border rounded-xl p-4">
				<h3 class="text-sm font-medium text-text-dim mb-3">Recent Ban Events</h3>
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead>
							<tr class="text-left text-text-dim border-b border-border">
								<th class="py-2 px-3 font-medium">Time</th>
								<th class="py-2 px-3 font-medium">Action</th>
								<th class="py-2 px-3 font-medium">Jail</th>
								<th class="py-2 px-3 font-medium">IP</th>
							</tr>
						</thead>
						<tbody>
							{#each banEvents as event}
								<tr class="border-b border-border/50">
									<td class="py-2 px-3 text-text-dim text-xs">{new Date(event.timestamp).toLocaleString()}</td>
									<td class="py-2 px-3">
										<span class="text-xs px-2 py-0.5 rounded {event.action === 'ban' ? 'bg-danger/15 text-danger' : 'bg-success/15 text-success'}">
											{event.action}
										</span>
									</td>
									<td class="py-2 px-3 font-mono text-xs">{event.jail}</td>
									<td class="py-2 px-3 font-mono text-xs">{event.ip}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}

		<!-- System logs -->
		<div class="bg-bg-card border border-border rounded-xl p-4">
			<div class="flex items-center justify-between mb-3">
				<h3 class="text-sm font-medium text-text-dim">System Logs</h3>
				<div class="flex gap-2">
					<input
						bind:value={logUnit}
						placeholder="Unit filter..."
						class="bg-bg border border-border rounded px-3 py-1.5 text-xs w-40 focus:outline-none focus:border-accent"
					/>
					<select
						bind:value={logPriority}
						onchange={filterLogs}
						class="bg-bg border border-border rounded px-3 py-1.5 text-xs focus:outline-none focus:border-accent"
					>
						<option value={-1}>All priorities</option>
						<option value={0}>Emergency</option>
						<option value={1}>Alert</option>
						<option value={2}>Critical</option>
						<option value={3}>Error</option>
						<option value={4}>Warning</option>
						<option value={5}>Notice</option>
						<option value={6}>Info</option>
						<option value={7}>Debug</option>
					</select>
					<button
						onclick={filterLogs}
						class="bg-accent/10 text-accent text-xs px-3 py-1.5 rounded hover:bg-accent/20 transition-colors cursor-pointer"
					>
						Filter
					</button>
				</div>
			</div>
			<div class="overflow-y-auto max-h-96 font-mono text-xs">
				{#each logs as entry}
					<div class="flex gap-3 py-1 border-b border-border/30 hover:bg-bg-hover">
						<span class="text-text-dim shrink-0 w-36">{formatTimestamp(entry.timestamp)}</span>
						<span class="shrink-0 w-16 {priorityColors[entry.priorityLabel] || 'text-text-dim'}">
							{entry.priorityLabel}
						</span>
						<span class="text-text-dim shrink-0 w-40 truncate">{entry.unit}</span>
						<span class="text-text break-all">{entry.message}</span>
					</div>
				{/each}
				{#if logs.length === 0}
					<div class="text-text-dim py-4 text-center">No logs matching filters</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
