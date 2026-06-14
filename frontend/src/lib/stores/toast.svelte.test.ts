import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { pushToast, dismissToast, toastError, getToasts } from './toast.svelte';

function drain(): void {
	for (const t of getToasts()) {
		dismissToast(t.id);
	}
}

describe('toast store', () => {
	beforeEach(() => {
		drain();
	});

	describe('toastError', () => {
		it('uses the Error message when given an Error', () => {
			toastError(new Error('boom'), 'fallback');
			const toasts = getToasts();
			expect(toasts).toHaveLength(1);
			expect(toasts[0].kind).toBe('error');
			expect(toasts[0].message).toBe('boom');
		});

		it('uses the fallback message for non-Error values', () => {
			toastError('a string', 'something failed');
			expect(getToasts()[0].message).toBe('something failed');
		});
	});

	describe('auto-dismiss timer', () => {
		beforeEach(() => {
			vi.useFakeTimers();
		});

		afterEach(() => {
			vi.useRealTimers();
		});

		it('removes the toast after the duration elapses', () => {
			pushToast('info', 'hello', 5000);
			expect(getToasts()).toHaveLength(1);

			vi.advanceTimersByTime(4999);
			expect(getToasts()).toHaveLength(1);

			vi.advanceTimersByTime(1);
			expect(getToasts()).toHaveLength(0);
		});

		it('never auto-dismisses when duration is zero', () => {
			pushToast('info', 'sticky', 0);
			vi.advanceTimersByTime(60000);
			expect(getToasts()).toHaveLength(1);
		});

		it('manual dismiss cancels the pending timer', () => {
			const id = pushToast('info', 'transient', 5000);
			dismissToast(id);
			expect(getToasts()).toHaveLength(0);

			// Advancing past the original duration must not throw or double-remove.
			vi.advanceTimersByTime(5000);
			expect(getToasts()).toHaveLength(0);
		});
	});
});
