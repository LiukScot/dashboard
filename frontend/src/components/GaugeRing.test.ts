import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import GaugeRing from './GaugeRing.svelte';

describe('GaugeRing', () => {
	it('renders label and value', () => {
		const { container } = render(GaugeRing, { value: 42.5, label: 'CPU' });
		expect(container.textContent).toContain('CPU');
		expect(container.textContent).toContain('42.5%');
	});

	it('rounds to one decimal place', () => {
		const { container } = render(GaugeRing, { value: 42.456, label: 'CPU' });
		// 42.456 → 42.5
		expect(container.textContent).toContain('42.5%');
	});

	it('clamps values above max', () => {
		const { container } = render(GaugeRing, { value: 250, label: 'CPU', max: 100 });
		// Should clamp to 100%, not show 250%.
		expect(container.textContent).toContain('100%');
		expect(container.textContent).not.toContain('250');
	});

	it('clamps negative values to zero', () => {
		const { container } = render(GaugeRing, { value: -10, label: 'CPU' });
		expect(container.textContent).toContain('0%');
	});

	it('shows subtitle when provided', () => {
		const { container } = render(GaugeRing, {
			value: 50,
			label: 'RAM',
			subtitle: '4 GB / 8 GB'
		});
		expect(container.textContent).toContain('4 GB / 8 GB');
	});

	it('omits subtitle when blank', () => {
		const { container } = render(GaugeRing, {
			value: 50,
			label: 'RAM',
			subtitle: ''
		});
		// No subtitle node should be present (no max-w-[9rem] class).
		expect(container.querySelector('.max-w-\\[9rem\\]')).toBeNull();
	});

	it('builds a label-derived gradient id with no spaces', () => {
		const { container } = render(GaugeRing, { value: 50, label: 'Network IO' });
		const gradient = container.querySelector('linearGradient');
		expect(gradient?.id).toBe('gauge-network-io');
	});
});
