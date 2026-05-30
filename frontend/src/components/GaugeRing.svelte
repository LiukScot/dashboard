<script lang="ts">
	interface Props {
		value: number;
		label: string;
		color?: string;
		max?: number;
		subtitle?: string;
	}

	const size = 140;
	const center = size / 2;
	const radius = 48;
	const circumference = 2 * Math.PI * radius;

	let { value, label, color = '#00d4aa', max = 100, subtitle = '' }: Props = $props();

	const safeMax = $derived(max > 0 ? max : 100);
	const clampedValue = $derived(Math.max(0, Math.min(value, safeMax)));
	const progress = $derived(clampedValue / safeMax);
	const dashOffset = $derived(circumference * (1 - progress));
	const valueLabel = $derived(Math.round(clampedValue * 10) / 10);
	const gradientId = $derived(`gauge-${label.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`);
	const glowId = $derived(`gauge-glow-${label.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`);

</script>

<div class="flex flex-col items-center">
	<svg viewBox={`0 0 ${size} ${size}`} class="w-40 h-36 overflow-visible">
		<defs>
			<linearGradient id={gradientId} x1="0%" y1="0%" x2="100%" y2="100%">
				<stop offset="0%" stop-color={color} stop-opacity="0.55" />
				<stop offset="100%" stop-color={color} />
			</linearGradient>
			<filter id={glowId} x="-50%" y="-50%" width="200%" height="200%">
				<feGaussianBlur stdDeviation="3" result="blur" />
				<feMerge>
					<feMergeNode in="blur" />
					<feMergeNode in="SourceGraphic" />
				</feMerge>
			</filter>
		</defs>

		<circle
			cx={center}
			cy={center}
			r={radius}
			fill="none"
			style="stroke: var(--color-border)"
			stroke-width="10"
		/>
		<circle
			cx={center}
			cy={center}
			r={radius}
			fill="none"
			stroke={`url(#${gradientId})`}
			stroke-width="10"
			stroke-linecap="round"
			stroke-dasharray={circumference}
			stroke-dashoffset={dashOffset}
			transform={`rotate(-90 ${center} ${center})`}
			filter={`url(#${glowId})`}
			style="transition: stroke-dashoffset 220ms ease-out;"
		/>

		<text
			x={center}
			y="60"
			text-anchor="middle"
			style="fill: var(--color-text)"
			font-size="11"
			font-weight="500"
			letter-spacing="0.04em"
		>
			{label}
		</text>
		<text
			x={center}
			y="84"
			text-anchor="middle"
			style="fill: var(--color-text)"
			font-size="21"
			font-weight="600"
		>
			{valueLabel}%
		</text>
	</svg>
	{#if subtitle}
		<span class="-mt-3 max-w-[9rem] text-center text-[11px] leading-4 text-text-dim">{subtitle}</span>
	{/if}
</div>
