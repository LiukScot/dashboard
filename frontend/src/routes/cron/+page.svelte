<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type CronOccurrence, type CronWeek } from '$lib/api';

	const hours = Array.from({ length: 24 }, (_, hour) => hour);
	const dayNames = ['MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT', 'SUN'];
	let loadToken = 0;

	let weekStart = $state(startOfWeek(new Date()));
	let week = $state<CronWeek | null>(null);
	let loading = $state(true);
	let error = $state('');
	let hideError = $state('');
	let selected = $state<CronOccurrence | null>(null);

	onMount(loadWeek);

	async function loadWeek() {
		const token = ++loadToken;
		const requestedWeekStart = weekStart;
		loading = true;
		error = '';
		hideError = '';
		try {
			const nextWeek = await api.cronWeek(toDateInput(requestedWeekStart));
			if (token !== loadToken) return;
			week = nextWeek;
			selected = nextWeek.occurrences[0] ?? null;
		} catch (err) {
			if (token !== loadToken) return;
			error = err instanceof Error ? err.message : 'Failed to load cron week';
			week = null;
			selected = null;
		} finally {
			if (token !== loadToken) return;
			loading = false;
		}
	}

	async function hideSelectedJob() {
		if (!selected) return;
		hideError = '';
		try {
			await api.hideCronJob(selected.jobId);
			await loadWeek();
		} catch (err) {
			hideError = err instanceof Error ? err.message : 'Failed to hide cron job';
		}
	}

	function moveWeek(days: number) {
		weekStart = addDays(weekStart, days);
		loadWeek();
	}

	function today() {
		weekStart = startOfWeek(new Date());
		loadWeek();
	}

	function daysInWeek() {
		return week?.days ?? Array.from({ length: 7 }, (_, index) => toDateInput(addDays(weekStart, index)));
	}

	function occurrencesForDay(dayKey: string) {
		if (!week) return [];
		return week.occurrences.filter((occurrence) => occurrence.dayKey === dayKey);
	}

	function occurrenceStyle(occurrence: CronOccurrence) {
		const top = (occurrence.minutesOfDay / 1440) * 100;
		return `top: ${top}%;`;
	}

	function occurrenceClass(occurrence: CronOccurrence) {
		if (occurrence.status === 'observed') return 'border-success/40 bg-success/15';
		if (occurrence.status === 'failed') return 'border-danger/40 bg-danger/15';
		if (occurrence.status === 'planned') return 'border-info/20 bg-info/10 opacity-75';
		return 'border-info/30 bg-info/15';
	}

	function shortCommand(command: string) {
		const parts = command.split(/\s+/);
		const first = parts[0]?.split('/').pop() || command;
		return parts.length > 1 ? `${first} ${parts.slice(1, 3).join(' ')}` : first;
	}

	function formatHour(hour: number) {
		return `${String(hour).padStart(2, '0')}:00`;
	}

	function formatTimeLabel(occurrence: CronOccurrence) {
		return occurrence.displayTime;
	}

	function formatScheduledLabel(occurrence: CronOccurrence) {
		return `${formatDate(occurrence.dayKey)}, ${occurrence.displayTime}`;
	}

	function formatDate(value: string) {
		const [year, month, day] = value.split('-').map(Number);
		return new Date(year, month - 1, day).toLocaleDateString([], { month: 'short', day: 'numeric' });
	}

	function weekTitle() {
		const end = addDays(weekStart, 6);
		return `${weekStart.toLocaleDateString([], { month: 'long', day: 'numeric' })} - ${end.toLocaleDateString([], { month: 'long', day: 'numeric' })}`;
	}

	function startOfWeek(value: Date) {
		const date = new Date(value);
		const day = date.getDay();
		const diff = day === 0 ? -6 : 1 - day;
		date.setHours(0, 0, 0, 0);
		date.setDate(date.getDate() + diff);
		return date;
	}

	function addDays(value: Date, days: number) {
		const date = new Date(value);
		date.setDate(date.getDate() + days);
		return date;
	}

	function toDateInput(value: Date) {
		const year = value.getFullYear();
		const month = String(value.getMonth() + 1).padStart(2, '0');
		const day = String(value.getDate()).padStart(2, '0');
		return `${year}-${month}-${day}`;
	}

	function warningLabel(count: number) {
		return `${count} cron source warning${count === 1 ? '' : 's'}`;
	}

	function warningCount() {
		return week?.warnings?.length ?? 0;
	}
</script>

<div class="space-y-5">
	<div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
		<div>
			<h2 class="text-lg font-semibold">Cron Weekly</h2>
			<p class="text-sm text-text-dim">{weekTitle()}</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<button class="rounded border border-border px-3 py-1.5 text-sm text-text-dim hover:bg-bg-hover" onclick={() => moveWeek(-7)}>
				‹
			</button>
			<button class="rounded border border-border px-3 py-1.5 text-sm text-text-dim hover:bg-bg-hover" onclick={today}>
				Today
			</button>
			<button class="rounded border border-border px-3 py-1.5 text-sm text-text-dim hover:bg-bg-hover" onclick={() => moveWeek(7)}>
				›
			</button>
			<button class="rounded bg-accent/10 px-3 py-1.5 text-sm text-accent hover:bg-accent/20" onclick={loadWeek}>
				Refresh
			</button>
			{#if week}
				<span class="rounded border border-border px-3 py-1.5 text-xs text-text-dim">
					history {week.historyCoverage}
				</span>
			{/if}
		</div>
	</div>

	{#if error}
		<div class="rounded border border-danger/40 bg-danger/10 p-4 text-sm text-danger">{error}</div>
	{:else if loading}
		<div class="flex items-center justify-center py-16">
			<div class="h-6 w-6 animate-spin rounded-full border-2 border-accent border-t-transparent"></div>
		</div>
		{:else if week}
			{#if warningCount() > 0}
			<details class="rounded border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
				<summary class="cursor-pointer list-none">
					<div class="flex items-start justify-between gap-3">
						<div>
							<div class="font-medium">{warningLabel(warningCount())}</div>
							<p class="mt-1 text-xs text-warning/80">
								Some cron files or history sources could not be read. Calendar may be incomplete.
							</p>
						</div>
						<span class="shrink-0 text-xs text-warning/80">Show details</span>
					</div>
				</summary>
				<ul class="mt-3 space-y-2 border-t border-warning/20 pt-3 text-xs text-warning/90">
					{#each week.warnings ?? [] as warning}
						<li class="break-words font-mono">{warning}</li>
					{/each}
				</ul>
			</details>
			{/if}
			{#if week.jobs.length === 0 && week.hiddenJobCount > 0}
				<div class="rounded border border-info/30 bg-info/10 p-3 text-sm text-info">
					All visible cron jobs are hidden. Reset them from Settings.
				</div>
			{/if}

		<div class="grid gap-4 xl:grid-cols-[1fr_320px]">
			<section class="overflow-hidden rounded-lg border border-border bg-bg-card">
				<div class="grid grid-cols-[58px_repeat(7,minmax(128px,1fr))] border-b border-border">
					<div class="border-r border-border"></div>
					{#each daysInWeek() as day, index}
						<div class="border-r border-border px-3 py-2 last:border-r-0 {day === toDateInput(new Date()) ? 'bg-accent/5' : ''}">
							<div class="text-xs font-semibold text-text-dim">{dayNames[index]}</div>
							<div class="text-xl font-semibold {day === toDateInput(new Date()) ? 'text-accent' : 'text-text'}">{formatDate(day)}</div>
						</div>
					{/each}
				</div>

				<div class="max-h-[calc(100vh-250px)] overflow-auto">
					<div class="grid min-w-[960px] grid-cols-[58px_repeat(7,minmax(128px,1fr))]">
						<div class="border-r border-border">
							{#each hours as hour}
								<div class="h-14 border-b border-border/70 pr-2 pt-1 text-right text-xs text-text-dim">
									{formatHour(hour)}
								</div>
							{/each}
						</div>

						{#each daysInWeek() as day}
							<div class="relative border-r border-border last:border-r-0">
								{#each hours as _hour}
									<div class="h-14 border-b border-border/70"></div>
								{/each}

								{#each occurrencesForDay(day) as occurrence}
									<button
										class="absolute left-1 right-1 min-h-10 rounded border px-2 py-1 text-left text-xs text-text shadow-sm transition hover:border-accent hover:bg-accent/15 {occurrenceClass(occurrence)}
											{selected?.id === occurrence.id ? 'border-accent bg-accent/20' : ''}
										"
										style={occurrenceStyle(occurrence)}
										onclick={() => (selected = occurrence)}
										title={occurrence.command}
									>
											<div class="font-mono text-text-dim">{formatTimeLabel(occurrence)}</div>
										<div class="truncate font-medium">{shortCommand(occurrence.command)}</div>
									</button>
								{/each}
							</div>
						{/each}
					</div>
				</div>
			</section>

			<aside class="rounded-lg border border-border bg-bg-card p-4">
				{#if selected}
					<div class="mb-4 flex items-start justify-between gap-3">
						<div>
							<div class="text-xs uppercase tracking-wide text-text-dim">Selected Job</div>
							<h3 class="mt-1 break-words text-sm font-semibold">{shortCommand(selected.command)}</h3>
						</div>
						<button
							class="rounded border border-border p-2 text-text-dim transition hover:border-warning/50 hover:bg-warning/10 hover:text-warning"
							onclick={hideSelectedJob}
							title="Hide this cron job"
							aria-label="Hide this cron job"
						>
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
								<path d="M3 3l18 18" />
								<path d="M10.6 10.6a2 2 0 0 0 2.8 2.8" />
								<path d="M9.9 4.2A10.7 10.7 0 0 1 12 4c5 0 9 4.5 10 8a12.5 12.5 0 0 1-2.5 4.1" />
								<path d="M6.5 6.6A12.4 12.4 0 0 0 2 12c1 3.5 5 8 10 8a10.8 10.8 0 0 0 4.3-.9" />
							</svg>
						</button>
					</div>
					{#if hideError}
						<p class="mb-4 rounded border border-danger/30 bg-danger/10 p-2 text-xs text-danger">{hideError}</p>
					{/if}
					<dl class="space-y-3 text-sm">
						<div>
							<dt class="text-xs text-text-dim">Scheduled</dt>
							<dd>{formatScheduledLabel(selected)}</dd>
						</div>
						<div>
							<dt class="text-xs text-text-dim">Command</dt>
							<dd class="break-words font-mono text-xs">{selected.command}</dd>
						</div>
						<div>
							<dt class="text-xs text-text-dim">User</dt>
							<dd>{selected.user || 'current crontab user'}</dd>
						</div>
						<div>
							<dt class="text-xs text-text-dim">Source</dt>
							<dd class="break-words font-mono text-xs">{selected.source}</dd>
						</div>
						<div>
							<dt class="text-xs text-text-dim">Status</dt>
							<dd class="capitalize">{selected.status}</dd>
						</div>
					</dl>
				{:else}
					<div class="py-8 text-center text-sm text-text-dim">
						{#if week.hiddenJobCount > 0}
							<p>All cron jobs for this view are hidden.</p>
							<p class="mt-2 text-xs">Reset hidden cron jobs in Settings.</p>
						{:else}
							<p>No cron jobs in this week.</p>
						{/if}
					</div>
				{/if}
			</aside>
		</div>
	{/if}
</div>
