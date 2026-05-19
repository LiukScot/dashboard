<script lang="ts">
	import type { Container } from '$lib/api';
	import type { ContainerStats } from '$lib/api';

	interface Props {
		containers: Container[];
		stats: ContainerStats[];
	}

	let { containers, stats }: Props = $props();

	const statsMap = $derived(new Map(stats.map((s) => [s.name, s])));

	function getStats(name: string): ContainerStats | undefined {
		return statsMap.get(name);
	}

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(1024));
		return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
	}

	const stateColors: Record<string, string> = {
		running: 'bg-success',
		exited: 'bg-danger',
		paused: 'bg-warning',
		restarting: 'bg-warning',
		created: 'bg-info'
	};
</script>

<div class="overflow-x-auto">
	<table class="w-full text-sm">
		<thead>
			<tr class="text-left text-text-dim border-b border-border">
				<th class="py-2 px-3 font-medium">Status</th>
				<th class="py-2 px-3 font-medium">Name</th>
				<th class="py-2 px-3 font-medium">Image</th>
				<th class="py-2 px-3 font-medium text-right">CPU</th>
				<th class="py-2 px-3 font-medium text-right">Memory</th>
				<th class="py-2 px-3 font-medium">Uptime</th>
			</tr>
		</thead>
		<tbody>
			{#each containers as container (container.id)}
				{@const s = getStats(container.name)}
				<tr class="border-b border-border/50 hover:bg-bg-hover transition-colors">
					<td class="py-2 px-3">
						<span class="inline-block w-2 h-2 rounded-full {stateColors[container.state] || 'bg-text-dim'}"></span>
					</td>
					<td class="py-2 px-3 font-mono text-xs">{container.name}</td>
					<td class="py-2 px-3 text-text-dim text-xs truncate max-w-48">{container.image}</td>
					<td class="py-2 px-3 text-right font-mono text-xs">
						{#if s}
							<span class="{s.cpuPercent > 80 ? 'text-danger' : s.cpuPercent > 50 ? 'text-warning' : 'text-text'}">
								{s.cpuPercent.toFixed(1)}%
							</span>
						{:else}
							<span class="text-text-dim">—</span>
						{/if}
					</td>
					<td class="py-2 px-3 text-right font-mono text-xs">
						{#if s}
							{formatBytes(s.memUsage)}
						{:else}
							<span class="text-text-dim">—</span>
						{/if}
					</td>
					<td class="py-2 px-3 text-text-dim text-xs">{container.status}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
