<script lang="ts">
	import { onMount } from 'svelte';
	import * as echarts from 'echarts/core';
	import { LineChart } from 'echarts/charts';
	import {
		GridComponent,
		TooltipComponent,
		LegendComponent
	} from 'echarts/components';
	import { CanvasRenderer } from 'echarts/renderers';

	echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer]);

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

	let { title, labels, series, yAxisLabel = '', height = '250px' }: Props = $props();

	let container: HTMLDivElement;
	let chart: echarts.ECharts;

	onMount(() => {
		chart = echarts.init(container, undefined, { renderer: 'canvas' });
		updateChart();

		const observer = new ResizeObserver(() => chart?.resize());
		observer.observe(container);

		return () => {
			observer.disconnect();
			chart?.dispose();
		};
	});

	function updateChart() {
		if (!chart) return;
		chart.setOption({
			backgroundColor: 'transparent',
			grid: { top: 40, right: 20, bottom: 30, left: 50 },
			tooltip: {
				trigger: 'axis',
				backgroundColor: '#1a1a26',
				borderColor: '#1e1e2e',
				textStyle: { color: '#e4e4ef', fontSize: 12 }
			},
			legend: {
				show: series.length > 1,
				top: 5,
				textStyle: { color: '#8888a0', fontSize: 11 }
			},
			title: {
				text: title,
				left: 10,
				top: 5,
				textStyle: { color: '#8888a0', fontSize: 12, fontWeight: 500 }
			},
			xAxis: {
				type: 'category',
				data: labels,
				axisLine: { lineStyle: { color: '#1e1e2e' } },
				axisLabel: { color: '#8888a0', fontSize: 10 },
				splitLine: { show: false }
			},
			yAxis: {
				type: 'value',
				name: yAxisLabel,
				nameTextStyle: { color: '#8888a0', fontSize: 10 },
				axisLine: { show: false },
				axisLabel: { color: '#8888a0', fontSize: 10 },
				splitLine: { lineStyle: { color: '#1e1e2e', type: 'dashed' } }
			},
			series: series.map((s) => ({
				name: s.name,
				type: 'line',
				smooth: true,
				symbol: 'none',
				lineStyle: { width: 2, color: s.color || '#00d4aa' },
				areaStyle: {
					color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
						{ offset: 0, color: (s.color || '#00d4aa') + '40' },
						{ offset: 1, color: (s.color || '#00d4aa') + '05' }
					])
				},
				data: s.data
			})),
			animation: true,
			animationDuration: 300
		});
	}

	$effect(() => {
		labels;
		series;
		updateChart();
	});
</script>

<div bind:this={container} style="width: 100%; height: {height};"></div>
