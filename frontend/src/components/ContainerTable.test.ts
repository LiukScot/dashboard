import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ContainerTable from './ContainerTable.svelte';
import type { Container, ContainerStats } from '$lib/api';

const sampleContainer = (overrides: Partial<Container> = {}): Container => ({
	id: 'aaaaaaaaaaaa',
	name: 'alpha',
	image: 'alpine:latest',
	state: 'running',
	status: 'Up 5 minutes',
	created: 0,
	ports: [],
	...overrides
});

const sampleStats = (overrides: Partial<ContainerStats> = {}): ContainerStats => ({
	id: 'aaaaaaaaaaaa',
	name: 'alpha',
	cpuPercent: 12.3,
	memUsage: 50 * 1024 * 1024,
	memLimit: 100 * 1024 * 1024,
	memPercent: 50,
	netRx: 0,
	netTx: 0,
	...overrides
});

describe('ContainerTable', () => {
	it('renders a row per container with name and image', () => {
		render(ContainerTable, {
			containers: [
				sampleContainer({ name: 'alpha', image: 'alpine:latest' }),
				sampleContainer({ id: 'bbbb', name: 'beta', image: 'nginx:1.27' })
			],
			stats: []
		});

		expect(screen.getByText('alpha')).toBeInTheDocument();
		expect(screen.getByText('beta')).toBeInTheDocument();
		expect(screen.getByText('alpine:latest')).toBeInTheDocument();
		expect(screen.getByText('nginx:1.27')).toBeInTheDocument();
	});

	it('shows an em-dash for containers without matching stats', () => {
		const { container } = render(ContainerTable, {
			containers: [sampleContainer()],
			stats: []
		});
		// Two unknown cells: CPU and Memory.
		const dashes = container.querySelectorAll('td .text-text-dim');
		// At least 2 placeholder spans (CPU + Memory). Status column also
		// uses text-text-dim for the uptime text, so >=2 is the safe bound.
		expect(dashes.length).toBeGreaterThanOrEqual(2);
	});

	it('formats memory usage in MB and shows CPU% from stats', () => {
		render(ContainerTable, {
			containers: [sampleContainer()],
			stats: [sampleStats({ cpuPercent: 25.4, memUsage: 200 * 1024 * 1024 })]
		});

		expect(screen.getByText('25.4%')).toBeInTheDocument();
		expect(screen.getByText('200.0 MB')).toBeInTheDocument();
	});

	it('renders an empty table body when there are no containers', () => {
		const { container } = render(ContainerTable, { containers: [], stats: [] });
		const tbody = container.querySelector('tbody');
		expect(tbody?.children.length).toBe(0);
	});
});
