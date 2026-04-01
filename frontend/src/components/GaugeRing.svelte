<script lang="ts">
	import { onMount } from 'svelte';
	import * as echarts from 'echarts/core';
	import { GaugeChart } from 'echarts/charts';
	import { CanvasRenderer } from 'echarts/renderers';

	echarts.use([GaugeChart, CanvasRenderer]);

	interface Props {
		value: number;
		label: string;
		color?: string;
		max?: number;
		subtitle?: string;
	}

	let { value, label, color = '#00d4aa', max = 100, subtitle = '' }: Props = $props();

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
			series: [
				{
					type: 'gauge',
					startAngle: 220,
					endAngle: -40,
					radius: '90%',
					center: ['50%', '55%'],
					min: 0,
					max,
					progress: {
						show: true,
						width: 12,
						roundCap: true,
						itemStyle: { color }
					},
					pointer: { show: false },
					axisLine: {
						lineStyle: {
							width: 12,
							color: [[1, '#1e1e2e']]
						},
						roundCap: true
					},
					axisTick: { show: false },
					splitLine: { show: false },
					axisLabel: { show: false },
					title: {
						show: true,
						offsetCenter: [0, '30%'],
						fontSize: 12,
						color: '#8888a0',
						fontFamily: 'Inter, system-ui, sans-serif'
					},
					detail: {
						valueAnimation: true,
						offsetCenter: [0, '-5%'],
						fontSize: 24,
						fontWeight: 600,
						formatter: '{value}%',
						color: '#e4e4ef',
						fontFamily: 'Inter, system-ui, sans-serif'
					},
					data: [{ value: Math.round(value * 10) / 10, name: label }]
				}
			]
		});
	}

	$effect(() => {
		value;
		updateChart();
	});
</script>

<div class="flex flex-col items-center">
	<div bind:this={container} class="w-40 h-36"></div>
	{#if subtitle}
		<span class="text-xs text-text-dim -mt-2">{subtitle}</span>
	{/if}
</div>
