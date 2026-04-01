<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, type Container, type SystemMetrics, type NetworkMetrics, type ContainerStats } from '$lib/api';
	import { subscribe, type MetricsMessage } from '$lib/ws';
	import GaugeRing from '../components/GaugeRing.svelte';
	import TimeChart from '../components/TimeChart.svelte';
	import ContainerTable from '../components/ContainerTable.svelte';

	let system = $state<SystemMetrics | null>(null);
	let network = $state<NetworkMetrics[]>([]);
	let containers = $state<Container[]>([]);
	let dockerStats = $state<ContainerStats[]>([]);
	let cpuHistory = $state<{ time: string; value: number }[]>([]);
	let netHistory = $state<{ time: string; rx: number; tx: number }[]>([]);

	let unsubscribeWs: (() => void) | null = null;

	onMount(async () => {
		// Initial fetch
		const [sys, cont, hist] = await Promise.all([
			api.systemOverview(),
			api.dockerContainers(),
			api.cpuHistory()
		]);
		system = sys;
		containers = cont;
		cpuHistory = hist.map((h) => ({
			time: new Date(h.timestamp).toLocaleTimeString(),
			value: h.cpuPercent
		}));

		// WebSocket for live updates
		unsubscribeWs = subscribe((msg: MetricsMessage) => {
			if (msg.system) {
				system = msg.system;
				cpuHistory = [
					...cpuHistory.slice(-119),
					{
						time: new Date(msg.system.timestamp).toLocaleTimeString(),
						value: msg.system.cpuPercent
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

	<!-- Gauges -->
	{#if system}
		<div class="grid grid-cols-2 md:grid-cols-4 gap-4">
			<div class="bg-bg-card border border-border rounded-xl p-4">
				<GaugeRing value={system.cpuPercent} label="CPU" color="#00d4aa"
					subtitle="{system.cpuCores} cores • Load {system.loadAvg[0].toFixed(2)}" />
			</div>
			<div class="bg-bg-card border border-border rounded-xl p-4">
				<GaugeRing value={system.memPercent} label="RAM" color="#4488ff"
					subtitle="{formatBytes(system.memUsed)} / {formatBytes(system.memTotal)}" />
			</div>
			<div class="bg-bg-card border border-border rounded-xl p-4">
				<GaugeRing value={system.diskPercent} label="Disk" color="#ffaa22"
					subtitle="{formatBytes(system.diskUsed)} / {formatBytes(system.diskTotal)}" />
			</div>
			<div class="bg-bg-card border border-border rounded-xl p-4">
				<GaugeRing
					value={system.swapTotal > 0 ? (system.swapUsed / system.swapTotal) * 100 : 0}
					label="Swap" color="#ff4466"
					subtitle="{formatBytes(system.swapUsed)} / {formatBytes(system.swapTotal)}" />
			</div>
		</div>
	{/if}

	<!-- Charts -->
	<div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
		<div class="bg-bg-card border border-border rounded-xl p-4">
			<TimeChart
				title="CPU Usage"
				labels={cpuHistory.map((h) => h.time)}
				series={[{ name: 'CPU %', data: cpuHistory.map((h) => Math.round(h.value * 10) / 10), color: '#00d4aa' }]}
				yAxisLabel="%"
			/>
		</div>
		<div class="bg-bg-card border border-border rounded-xl p-4">
			<TimeChart
				title="Network Bandwidth"
				labels={netHistory.map((h) => h.time)}
				series={[
					{ name: 'Download', data: netHistory.map((h) => Math.round(h.rx * 10) / 10), color: '#4488ff' },
					{ name: 'Upload', data: netHistory.map((h) => Math.round(h.tx * 10) / 10), color: '#00d4aa' }
				]}
				yAxisLabel="KB/s"
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
		<ContainerTable {containers} stats={dockerStats} />
	</div>
</div>
