<script lang="ts">
	import { onMount } from 'svelte';
	import { graphic, init, use, type EChartsType } from 'echarts/core';
	import { LineChart } from 'echarts/charts';
	import { GridComponent, LegendComponent, TitleComponent, TooltipComponent } from 'echarts/components';
	import { CanvasRenderer } from 'echarts/renderers';

	use([LineChart, GridComponent, LegendComponent, TitleComponent, TooltipComponent, CanvasRenderer]);

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

	const fallbackHeight = 250;

	let { title, labels, series, yAxisLabel = '', height = '250px' }: Props = $props();
	let container: HTMLDivElement | undefined;
	let chart: EChartsType | null = null;
	let resizeObserver: ResizeObserver | null = null;

	const safeSeries = $derived(series.filter((item) => item.data.length > 0));
	const maxValue = $derived.by(() => {
		const values = safeSeries.flatMap((item) => item.data);
		const highest = values.length > 0 ? Math.max(...values) : 0;
		if (highest <= 0) return 1;
		return Math.ceil(highest * 1.1 * 10) / 10;
	});

	function parseHeight(input: string): number {
		const parsed = Number.parseInt(input, 10);
		return Number.isFinite(parsed) ? parsed : fallbackHeight;
	}

	function toRgba(hexColor: string, alpha: number): string {
		const match = /^#?([\da-f]{2})([\da-f]{2})([\da-f]{2})$/i.exec(hexColor);
		if (!match) return `rgba(0, 212, 170, ${alpha})`;

		const [, red, green, blue] = match;
		return `rgba(${parseInt(red, 16)}, ${parseInt(green, 16)}, ${parseInt(blue, 16)}, ${alpha})`;
	}

	function formatYAxisValue(value: number): string {
		if (maxValue >= 100) return `${Math.round(value)}`;
		if (maxValue >= 10) return `${value.toFixed(1)}`;
		return `${value.toFixed(2)}`;
	}

	function updateChart() {
		if (!chart) return;

		chart.setOption(
			{
				backgroundColor: 'transparent',
				animation: true,
				title: {
					text: title,
					left: 12,
					top: 8,
					textStyle: {
						color: '#8888a0',
						fontSize: 12,
						fontWeight: 500
					}
				},
				legend: {
					show: safeSeries.length > 1,
					top: 8,
					right: 12,
					icon: 'circle',
					itemWidth: 8,
					itemHeight: 8,
					textStyle: {
						color: '#8888a0',
						fontSize: 11
					}
				},
				grid: {
					top: safeSeries.length > 1 ? 48 : 38,
					right: 18,
					bottom: 30,
					left: 42
				},
				tooltip: {
					trigger: 'axis',
					backgroundColor: '#12121a',
					borderColor: '#1e1e2e',
					textStyle: {
						color: '#e4e4ef'
					},
					axisPointer: {
						type: 'line',
						lineStyle: {
							color: '#8888a0',
							opacity: 0.35
						}
					}
				},
				xAxis: {
					type: 'category',
					data: labels,
					boundaryGap: false,
					axisLine: {
						lineStyle: {
							color: '#1e1e2e'
						}
					},
					axisTick: {
						show: false
					},
					axisLabel: {
						color: '#8888a0',
						fontSize: 10,
						hideOverlap: true
					}
				},
				yAxis: {
					type: 'value',
					name: yAxisLabel,
					max: maxValue,
					splitNumber: 4,
					nameTextStyle: {
						color: '#8888a0',
						fontSize: 10,
						padding: [0, 0, 4, 0]
					},
					axisLine: {
						show: false
					},
					axisTick: {
						show: false
					},
					axisLabel: {
						color: '#8888a0',
						fontSize: 10,
						formatter: (value: number) => formatYAxisValue(value)
					},
					splitLine: {
						lineStyle: {
							color: '#1e1e2e',
							type: 'dashed'
						}
					}
				},
				series: safeSeries.map((item) => {
					const color = item.color || '#00d4aa';
					return {
						name: item.name,
						type: 'line',
						smooth: true,
						showSymbol: false,
						data: item.data,
						lineStyle: {
							color,
							width: 3
						},
						areaStyle: {
							color: new graphic.LinearGradient(0, 0, 0, 1, [
								{ offset: 0, color: toRgba(color, 0.28) },
								{ offset: 1, color: toRgba(color, 0.03) }
							])
						},
						emphasis: {
							focus: 'series'
						}
					};
				})
			},
			{ notMerge: true }
		);
	}

	onMount(() => {
		if (!container) return;

		chart = init(container, undefined, { renderer: 'canvas' });
		resizeObserver = new ResizeObserver(() => chart?.resize());
		resizeObserver.observe(container);
		updateChart();

		return () => {
			resizeObserver?.disconnect();
			resizeObserver = null;
			chart?.dispose();
			chart = null;
		};
	});

	$effect(() => {
		title;
		labels;
		safeSeries;
		yAxisLabel;
		height;
		maxValue;

		updateChart();
		chart?.resize();
	});
</script>

<div class="w-full" style={`height: ${height};`}>
	<div bind:this={container} class="h-full w-full"></div>
</div>
