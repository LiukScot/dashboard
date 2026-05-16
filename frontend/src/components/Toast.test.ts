import { describe, it, expect, beforeEach } from 'vitest';
import { flushSync } from 'svelte';
import { render, screen } from '@testing-library/svelte';
import Toast from './Toast.svelte';
import { pushToast, dismissToast, getToasts } from '$lib/stores/toast.svelte';

describe('Toast component', () => {
	beforeEach(() => {
		// Drain any toasts left over from the previous test.
		for (const t of getToasts()) {
			dismissToast(t.id);
		}
	});

	it('renders nothing when there are no toasts', () => {
		render(Toast);
		// Both live regions exist (a11y), but contain no toast bodies.
		const regions = screen.getAllByRole('region');
		expect(regions).toHaveLength(2);
		for (const r of regions) {
			expect(r.children).toHaveLength(0);
		}
	});

	it('shows pushed toast in polite region', () => {
		render(Toast);
		pushToast('info', 'hello world', 0);
		flushSync();

		const polite = screen.getByRole('region', { name: 'Notifications' });
		expect(polite.textContent).toContain('hello world');
	});

	it('routes error toasts to the assertive region', () => {
		render(Toast);
		pushToast('error', 'boom', 0);
		flushSync();

		const assertive = screen.getByRole('region', { name: 'Errors' });
		expect(assertive.textContent).toContain('boom');

		const polite = screen.getByRole('region', { name: 'Notifications' });
		expect(polite.textContent).not.toContain('boom');
	});
});
