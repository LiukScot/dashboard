<script lang="ts">
	interface Series {
		name: string;
		data: number[];
		color?: string;
	}

	interface Props {
		title: string;
		labels: string[];
		series: Series[];
		yAxisLabel?: string;
		height?: string;
	}

	const chartWidth = 640;
	const fallbackHeight = 250;
	const marginTop = 34;
	const marginRight = 18;
	const marginBottom = 36;
	const marginLeft = 42;

	let { title, labels, series, yAxisLabel = '', height = '250px' }: Props = $props();

	function parseHeight(input: string): number {
		const parsed = Number.parseInt(input, 10);
		return Number.isFinite(parsed) ? parsed : fallbackHeight;
	}

	const chartHeight = $derived(parseHeight(height));
	const plotWidth = $derived(chartWidth - marginLeft - marginRight);
	const plotHeight = $derived(chartHeight - marginTop - marginBottom);
	const safeSeries = $derived(series.filter((item) => item.data.length > 0));
	const maxValue = $derived.by(() => {
		const values = safeSeries.flatMap((item) => item.data);
		const highest = values.length > 0 ? Math.max(...values) : 0;
		return highest > 0 ? highest : 1;
	});
	const xDivisor = $derived(Math.max(labels.length - 1, 1));
	const yTicks = $derived(Array.from({ length: 5 }, (_, index) => index / 4));
	const xTickIndexes = $derived.by(() => {
		if (labels.length <= 6) {
			return labels.map((_, index) => index);
		}

		return Array.from({ length: 6 }, (_, index) =>
			Math.min(labels.length - 1, Math.round((index / 5) * (labels.length - 1)))
		);
	});

	function xFor(index: number): number {
		return marginLeft + (index / xDivisor) * plotWidth;
	}

	function yFor(value: number): number {
		return marginTop + plotHeight - (value / maxValue) * plotHeight;
	}

	function linePath(data: number[]): string {
		return data.map((point, index) => `${index === 0 ? 'M' : 'L'} ${xFor(index)} ${yFor(point)}`).join(' ');
	}

	function areaPath(data: number[]): string {
		if (data.length === 0) return '';
		const line = linePath(data);
		return `${line} L ${xFor(data.length - 1)} ${marginTop + plotHeight} L ${xFor(0)} ${marginTop + plotHeight} Z`;
	}

	function formatYAxisValue(value: number): string {
		if (maxValue >= 100) return `${Math.round(value)}`;
		if (maxValue >= 10) return `${value.toFixed(1)}`;
		return `${value.toFixed(2)}`;
	}

	function gradientId(index: number): string {
		return `series-gradient-${title.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-${index}`;
	}
</script>

<div class="w-full" style={`height: ${height};`}>
	<svg viewBox={`0 0 ${chartWidth} ${chartHeight}`} class="h-full w-full overflow-visible">
		<defs>
			{#each safeSeries as item, index}
				<linearGradient id={gradientId(index)} x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" stop-color={item.color || '#00d4aa'} stop-opacity="0.32" />
					<stop offset="100%" stop-color={item.color || '#00d4aa'} stop-opacity="0.03" />
				</linearGradient>
			{/each}
		</defs>

		<text x="12" y="18" class="fill-text-dim text-[12px] font-medium">{title}</text>
		{#if yAxisLabel}
			<text x={marginLeft - 30} y={marginTop - 10} class="fill-text-dim text-[10px]">{yAxisLabel}</text>
		{/if}

		{#each yTicks as tick}
			{@const y = marginTop + plotHeight - tick * plotHeight}
			<line x1={marginLeft} y1={y} x2={marginLeft + plotWidth} y2={y} stroke="#1e1e2e" stroke-dasharray="3 6" />
			<text x={marginLeft - 8} y={y + 4} text-anchor="end" class="fill-text-dim text-[10px]">
				{formatYAxisValue(tick * maxValue)}
			</text>
		{/each}

		<line
			x1={marginLeft}
			y1={marginTop + plotHeight}
			x2={marginLeft + plotWidth}
			y2={marginTop + plotHeight}
			stroke="#1e1e2e"
		/>

		{#each safeSeries as item, index}
			<path d={areaPath(item.data)} fill={`url(#${gradientId(index)})`} />
			<path
				d={linePath(item.data)}
				fill="none"
				stroke={item.color || '#00d4aa'}
				stroke-width="2.5"
				stroke-linecap="round"
				stroke-linejoin="round"
			/>
		{/each}

		{#each xTickIndexes as tickIndex}
			<text x={xFor(tickIndex)} y={chartHeight - 10} text-anchor="middle" class="fill-text-dim text-[10px]">
				{labels[tickIndex]}
			</text>
		{/each}

		{#if safeSeries.length > 1}
			{#each safeSeries as item, index}
				<circle cx={chartWidth - 118 + index * 80} cy="16" r="4" fill={item.color || '#00d4aa'} />
				<text x={chartWidth - 108 + index * 80} y="20" class="fill-text-dim text-[11px]">{item.name}</text>
			{/each}
		{/if}
	</svg>
</div>
