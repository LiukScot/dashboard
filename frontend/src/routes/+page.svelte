<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		api,
		type Container,
		type SystemMetrics,
		type NetworkMetrics,
		type ContainerStats,
		type HistoryRange,
		type HistorySample
	} from '$lib/api';
	import { subscribe, type MetricsMessage } from '$lib/ws';
	import GaugeRing from '../components/GaugeRing.svelte';
	import TimeChart from '../components/TimeChart.svelte';
	import ContainerTable from '../components/ContainerTable.svelte';

	let system = $state<SystemMetrics | null>(null);
	let network = $state<NetworkMetrics[]>([]);
	let containers = $state<Container[]>([]);
	let dockerStats = $state<ContainerStats[]>([]);
	let dockerError = $state('');
	type LivePoint = { time: string; cpu: number; mem: number; disk: number; swap: number };
	let liveHistory = $state<LivePoint[]>([]);
	let netHistory = $state<{ time: string; rx: number; tx: number }[]>([]);

	type MetricKey = 'cpu' | 'mem' | 'disk' | 'swap';
	const metrics: Record<MetricKey, {
		label: string;
		color: string;
		live: (p: LivePoint) => number;
		hist: (s: HistorySample) => number;
	}> = {
		cpu:  { label: 'CPU',  color: '#00d4aa', live: (p) => p.cpu,  hist: (s) => s.cpuPercent },
		mem:  { label: 'RAM',  color: '#4488ff', live: (p) => p.mem,  hist: (s) => s.memPercent },
		disk: { label: 'Disk', color: '#ffaa22', live: (p) => p.disk, hist: (s) => s.diskPercent },
		swap: { label: 'Swap', color: '#ff4466', live: (p) => p.swap, hist: (s) => s.swapPercent }
	};
	const metricKeys: MetricKey[] = ['cpu', 'mem', 'disk', 'swap'];
	let selectedMetric = $state<MetricKey>('cpu');

	function swapPercent(s: SystemMetrics | null): number {
		if (!s || s.swapTotal <= 0) return 0;
		return (s.swapUsed / s.swapTotal) * 100;
	}

	const historyRanges: { value: HistoryRange; label: string }[] = [
		{ value: '1h', label: '1h' },
		{ value: '6h', label: '6h' },
		{ value: '24h', label: '24h' },
		{ value: '7d', label: '7d' },
		{ value: '30d', label: '30d' }
	];
	let historyRange = $state<HistoryRange>('24h');
	let historyLive = $state(true);
	let historySamples = $state<HistorySample[]>([]);
	let historyError = $state('');
	let historyLoading = $state(false);

	const metricChartLabels = $derived(
		historyLive
			? liveHistory.map((h) => h.time)
			: historySamples.map((s) => formatHistoryLabel(s.timestamp, historyRange))
	);
	const metricChartSeries = $derived.by(() => {
		const m = metrics[selectedMetric];
		const data = historyLive
			? liveHistory.map((p) => Math.round(m.live(p) * 10) / 10)
			: historySamples.map((s) => Math.round(m.hist(s) * 10) / 10);
		return [{ name: `${m.label} %`, data, color: m.color }];
	});
	const netChartLabels = $derived(
		historyLive
			? netHistory.map((h) => h.time)
			: historySamples.map((s) => formatHistoryLabel(s.timestamp, historyRange))
	);
	const netChartSeries = $derived(
		historyLive
			? [
					{
						name: 'Download',
						data: netHistory.map((h) => Math.round(h.rx * 10) / 10),
						color: '#4488ff'
					},
					{
						name: 'Upload',
						data: netHistory.map((h) => Math.round(h.tx * 10) / 10),
						color: '#00d4aa'
					}
				]
			: [
					{
						name: 'Download',
						data: historySamples.map((s) => Math.round((s.netRxRate / 1024) * 10) / 10),
						color: '#4488ff'
					},
					{
						name: 'Upload',
						data: historySamples.map((s) => Math.round((s.netTxRate / 1024) * 10) / 10),
						color: '#00d4aa'
					}
				]
	);

	let unsubscribeWs: (() => void) | null = null;

	async function loadHistory(range: HistoryRange) {
		historyLoading = true;
		historyError = '';
		try {
			historySamples = await api.systemHistory(range);
		} catch (err) {
			historySamples = [];
			historyError = err instanceof Error ? err.message : 'Failed to load history';
		} finally {
			historyLoading = false;
		}
	}

	function selectRange(r: HistoryRange) {
		historyRange = r;
		historyLive = false;
		void loadHistory(r);
	}

	function selectLive() {
		historyLive = true;
		historyError = '';
	}

	function formatHistoryLabel(ts: number, range: HistoryRange): string {
		const d = new Date(ts * 1000);
		if (range === '1h' || range === '6h' || range === '24h') {
			return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
		}
		return d.toLocaleString([], { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' });
	}

	onMount(async () => {
		const [sys, hist, netSeed] = await Promise.all([
			api.systemOverview(),
			api.cpuHistory(),
			api.systemHistory('1h').catch(() => [] as HistorySample[])
		]);
		system = sys;
		liveHistory = hist.map((h) => ({
			time: new Date(h.timestamp).toLocaleTimeString(),
			cpu: h.cpuPercent,
			mem: h.memPercent,
			disk: h.diskPercent,
			swap: h.swapTotal > 0 ? (h.swapUsed / h.swapTotal) * 100 : 0
		}));
		// Seed live network buffer from persisted history so chart isn't empty on reload.
		netHistory = netSeed.slice(-60).map((s) => ({
			time: new Date(s.timestamp * 1000).toLocaleTimeString(),
			rx: s.netRxRate / 1024,
			tx: s.netTxRate / 1024
		}));

		try {
			containers = await api.dockerContainers();
			dockerError = '';
		} catch (err) {
			containers = [];

			dockerError = err instanceof Error ? err.message : 'Failed to load containers';
		}

		// WebSocket for live updates
		unsubscribeWs = subscribe((msg: MetricsMessage) => {
			if (msg.system) {
				system = msg.system;
				const s = msg.system;
				liveHistory = [
					...liveHistory.slice(-119),
					{
						time: new Date(s.timestamp).toLocaleTimeString(),
						cpu: s.cpuPercent,
						mem: s.memPercent,
						disk: s.diskPercent,
						swap: s.swapTotal > 0 ? (s.swapUsed / s.swapTotal) * 100 : 0
					}
				];
			}
			if (msg.network) {
				network = msg.network;
				const total = msg.network.reduce(
					(acc, n) => ({ rx: acc.rx + n.rxRate, tx: acc.tx + n.txRate }),
					{ rx: 0, tx: 0 }
				);
				netHistory = [
					...netHistory.slice(-59),
					{
						time: new Date().toLocaleTimeString(),
						rx: total.rx / 1024,
						tx: total.tx / 1024
					}
				];
			}
			if (msg.docker) dockerStats = msg.docker;
		});
	});

	onDestroy(() => {
		unsubscribeWs?.();
	});

	function formatUptime(seconds: number): string {
		const d = Math.floor(seconds / 86400);
		const h = Math.floor((seconds % 86400) / 3600);
		const m = Math.floor((seconds % 3600) / 60);
		if (d > 0) return `${d}d ${h}h ${m}m`;
		if (h > 0) return `${h}h ${m}m`;
		return `${m}m`;
	}

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(1024));
		return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-lg font-semibold">System Overview</h2>
			{#if system}
				<p class="text-sm text-text-dim">{system.hostname}</p>
			{/if}
		</div>
		{#if system}
			<div class="text-sm text-text-dim">
				Uptime: <span class="text-text">{formatUptime(system.uptime)}</span>
			</div>
		{/if}
	</div>

	<!-- Gauges (clickable: pick metric to drive chart) -->
	{#if system}
		{@const sys = system}
		{@const gauges: { key: MetricKey; value: number; subtitle: string }[] = [
			{ key: 'cpu',  value: sys.cpuPercent,  subtitle: `${sys.cpuCores} cores • Load ${sys.loadAvg[0].toFixed(2)}` },
			{ key: 'mem',  value: sys.memPercent,  subtitle: `${formatBytes(sys.memUsed)} / ${formatBytes(sys.memTotal)}` },
			{ key: 'disk', value: sys.diskPercent, subtitle: `${formatBytes(sys.diskUsed)} / ${formatBytes(sys.diskTotal)}` },
			{ key: 'swap', value: swapPercent(sys), subtitle: `${formatBytes(sys.swapUsed)} / ${formatBytes(sys.swapTotal)}` }
		]}
		<div class="grid grid-cols-2 md:grid-cols-4 gap-4">
			{#each gauges as g (g.key)}
				<button
					type="button"
					aria-pressed={selectedMetric === g.key}
					onclick={() => (selectedMetric = g.key)}
					class="bg-bg-card border rounded-xl p-4 text-left transition-colors cursor-pointer
						{selectedMetric === g.key
							? 'border-accent ring-1 ring-accent/40'
							: 'border-border hover:border-text-dim'}"
				>
					<GaugeRing
						value={g.value}
						label={metrics[g.key].label}
						color={metrics[g.key].color}
						subtitle={g.subtitle}
					/>
				</button>
			{/each}
		</div>
	{/if}

	<!-- Range selector -->
	<div class="flex flex-wrap items-center gap-2 text-sm">
		<span class="text-text-dim mr-1">Range:</span>
		<button
			type="button"
			class="px-3 py-1 rounded-md border transition-colors {historyLive
				? 'border-accent text-accent bg-accent/10'
				: 'border-border text-text-dim hover:text-text hover:border-text-dim'}"
			onclick={selectLive}
		>
			Live
		</button>
		{#each historyRanges as r (r.value)}
			<button
				type="button"
				class="px-3 py-1 rounded-md border transition-colors {!historyLive && historyRange === r.value
					? 'border-accent text-accent bg-accent/10'
					: 'border-border text-text-dim hover:text-text hover:border-text-dim'}"
				onclick={() => selectRange(r.value)}
			>
				{r.label}
			</button>
		{/each}
		{#if historyLoading}
			<span class="text-text-dim ml-2">Loading…</span>
		{/if}
		{#if historyError}
			<span class="text-danger ml-2">{historyError}</span>
		{/if}
	</div>

	<!-- Charts -->
	<div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
		<div class="bg-bg-card border border-border rounded-xl p-4">
			<TimeChart
				title={`${metrics[selectedMetric].label} ${historyLive ? '(live)' : `(${historyRange})`}`}
				labels={metricChartLabels}
				series={metricChartSeries}
				yAxisLabel="%"
				zoomable={!historyLive}
			/>
		</div>
		<div class="bg-bg-card border border-border rounded-xl p-4">
			<TimeChart
				title={historyLive ? 'Network Bandwidth (live)' : `Network Bandwidth (${historyRange})`}
				labels={netChartLabels}
				series={netChartSeries}
				yAxisLabel="KB/s"
				zoomable={!historyLive}
			/>
		</div>
	</div>

	<!-- Docker containers -->
	<div class="bg-bg-card border border-border rounded-xl p-4">
		<h3 class="text-sm font-medium text-text-dim mb-3">
			Docker Containers
			<span class="text-accent ml-2">
				{containers.filter((c) => c.state === 'running').length} running
			</span>
			<span class="text-text-dim ml-1">/ {containers.length} total</span>
		</h3>
		{#if dockerError}
			<p class="mb-3 text-sm text-danger">{dockerError}</p>
		{/if}
		<ContainerTable {containers} stats={dockerStats} />
	</div>
</div>
