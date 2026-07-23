import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { subscribe, subscribeState, getState, type MetricsMessage } from './ws';

// Minimal WebSocket double. The real ws module only touches readyState, the
// onopen/onmessage/onclose/onerror handlers, and close(); jsdom ships no
// WebSocket, so we stand in a controllable fake and drive the handlers
// manually to simulate the connection lifecycle.
class FakeWebSocket {
	static instances: FakeWebSocket[] = [];
	static readonly CONNECTING = 0;
	static readonly OPEN = 1;
	static readonly CLOSING = 2;
	static readonly CLOSED = 3;

	readyState = FakeWebSocket.CONNECTING;
	url: string;
	onopen: (() => void) | null = null;
	onmessage: ((ev: { data: string }) => void) | null = null;
	onclose: (() => void) | null = null;
	onerror: (() => void) | null = null;
	close = vi.fn(() => {
		this.readyState = FakeWebSocket.CLOSED;
	});

	constructor(url: string) {
		this.url = url;
		FakeWebSocket.instances.push(this);
	}

	open() {
		this.readyState = FakeWebSocket.OPEN;
		this.onopen?.();
	}

	emit(msg: MetricsMessage) {
		this.onmessage?.({ data: JSON.stringify(msg) });
	}

	serverClose() {
		this.readyState = FakeWebSocket.CLOSED;
		this.onclose?.();
	}

	static last(): FakeWebSocket {
		return FakeWebSocket.instances[FakeWebSocket.instances.length - 1];
	}
}

describe('ws client', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		FakeWebSocket.instances = [];
		vi.stubGlobal('WebSocket', FakeWebSocket);
	});

	afterEach(() => {
		vi.runOnlyPendingTimers();
		vi.useRealTimers();
		vi.unstubAllGlobals();
	});

	it('opens a socket on first subscribe and reports connected on open', () => {
		const states: string[] = [];
		const stopState = subscribeState((s) => states.push(s));

		const stop = subscribe(() => {});
		expect(FakeWebSocket.instances).toHaveLength(1);

		// Derives ws/wss from the page protocol and targets /ws.
		expect(FakeWebSocket.last().url).toMatch(/^wss?:\/\/[^/]+\/ws$/);
		expect(getState()).toBe('connecting');

		FakeWebSocket.last().open();
		expect(getState()).toBe('connected');
		expect(states).toContain('connecting');
		expect(states).toContain('connected');

		stop();
		stopState();
	});

	it('delivers parsed metrics frames to listeners', () => {
		const received: MetricsMessage[] = [];
		const stop = subscribe((msg) => received.push(msg));

		FakeWebSocket.last().open();
		FakeWebSocket.last().emit({ type: 'metrics', system: undefined });

		expect(received).toHaveLength(1);
		expect(received[0].type).toBe('metrics');

		stop();
	});

	it('does not throw on a malformed frame and keeps the connection', () => {
		const received: MetricsMessage[] = [];
		const stop = subscribe((msg) => received.push(msg));
		const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

		FakeWebSocket.last().open();
		FakeWebSocket.last().onmessage?.({ data: 'not json{' });

		expect(received).toHaveLength(0);
		expect(errSpy).toHaveBeenCalled();

		errSpy.mockRestore();
		stop();
	});

	it('reconnects with backoff after an unexpected close while subscribed', () => {
		const stop = subscribe(() => {});
		FakeWebSocket.last().open();
		expect(FakeWebSocket.instances).toHaveLength(1);

		FakeWebSocket.last().serverClose();
		expect(getState()).toBe('disconnected');

		// Backoff is scheduled, not immediate.
		expect(FakeWebSocket.instances).toHaveLength(1);
		vi.advanceTimersByTime(1000);
		expect(FakeWebSocket.instances).toHaveLength(2);

		stop();
	});

	it('does not reconnect after the last listener unsubscribes', () => {
		const stop = subscribe(() => {});
		FakeWebSocket.last().open();

		stop();
		// Unsubscribe closes the socket; its onclose must not schedule a retry.
		FakeWebSocket.last().serverClose();
		vi.advanceTimersByTime(60000);

		expect(FakeWebSocket.instances).toHaveLength(1);
		expect(getState()).toBe('disconnected');
	});
});
