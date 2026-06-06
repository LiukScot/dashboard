<script lang="ts">
	import { onMount } from 'svelte';
	import { graphic, init, use, type EChartsType } from 'echarts/core';
	import { LineChart } from 'echarts/charts';
	import {
		DataZoomComponent,
		GridComponent,
		LegendComponent,
		TitleComponent,
		TooltipComponent
	} from 'echarts/components';
	import { CanvasRenderer } from 'echarts/renderers';

	use([
		LineChart,
		GridComponent,
		LegendComponent,
		TitleComponent,
		TooltipComponent,
		DataZoomComponent,
		CanvasRenderer
	]);

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
		zoomable?: boolean;
	}

	const fallbackHeight = 250;

	// Colors mirroring CSS design tokens; ECharts config is pure JS so CSS
	// variables cannot be used directly — keep values in sync with app.css.
	const THEME = {
		border:  '#1e1e2e',
		textDim: '#8888a0',
		text:    '#e4e4ef',
		bgCard:  '#12121a',
	} as const;

	let { title, labels, series, yAxisLabel = '', height = '250px', zoomable = false }: Props = $props();
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
					text: yAxisLabel ? `${title} · ${yAxisLabel}` : title,
					left: 12,
					top: 8,
					textStyle: {
						color: THEME.textDim,
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
						color: THEME.textDim,
						fontSize: 11
					}
				},
				grid: {
					top: safeSeries.length > 1 ? 48 : 38,
					right: 18,
					bottom: zoomable ? 56 : 30,
					left: 42
				},
				dataZoom: zoomable
					? [
							{ type: 'inside', start: 0, end: 100 },
							{
								type: 'slider',
								height: 18,
								bottom: 8,
								borderColor: THEME.border,
								backgroundColor: 'transparent',
								fillerColor: 'rgba(0, 212, 170, 0.15)',
								handleStyle: { color: '#00d4aa' },
								moveHandleStyle: { color: '#00d4aa' },
								textStyle: { color: THEME.textDim, fontSize: 10 },
								dataBackground: {
									lineStyle: { color: THEME.border },
									areaStyle: { color: THEME.border }
								},
								selectedDataBackground: {
									lineStyle: { color: '#00d4aa' },
									areaStyle: { color: 'rgba(0, 212, 170, 0.18)' }
								}
							}
						]
					: undefined,
				tooltip: {
					trigger: 'axis',
					backgroundColor: THEME.bgCard,
					borderColor: THEME.border,
					textStyle: {
						color: THEME.text
					},
					axisPointer: {
						type: 'line',
						lineStyle: {
							color: THEME.textDim,
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
							color: THEME.border
						}
					},
					axisTick: {
						show: false
					},
					axisLabel: {
						color: THEME.textDim,
						fontSize: 10,
						hideOverlap: true
					}
				},
				yAxis: {
					type: 'value',
					max: maxValue,
					splitNumber: 4,
					axisLine: {
						show: false
					},
					axisTick: {
						show: false
					},
					axisLabel: {
						color: THEME.textDim,
						fontSize: 10,
						formatter: (value: number) => formatYAxisValue(value)
					},
					splitLine: {
						lineStyle: {
							color: THEME.border,
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
			{ replaceMerge: ['series'] }
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
		zoomable;

		updateChart();
	});
</script>

<div class="w-full" style={`height: ${height};`}>
	<div bind:this={container} class="h-full w-full"></div>
</div>
