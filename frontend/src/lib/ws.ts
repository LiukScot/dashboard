import type { SystemMetrics, NetworkMetrics, ContainerStats } from './api';

export interface MetricsMessage {
	type: 'metrics';
	system?: SystemMetrics;
	network?: NetworkMetrics[];
	docker?: ContainerStats[];
}

export type WsState = 'connecting' | 'connected' | 'disconnected';

type Listener = (msg: MetricsMessage) => void;
type StateListener = (state: WsState) => void;

let socket: WebSocket | null = null;
const listeners: Set<Listener> = new Set();
const stateListeners: Set<StateListener> = new Set();
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let currentState: WsState = 'disconnected';

const BASE_DELAY_MS = 1000;
const MAX_DELAY_MS = 30000;

function getWsUrl(): string {
	const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
	return `${proto}//${location.host}/ws`;
}

function setState(next: WsState): void {
	if (currentState === next) return;
	currentState = next;
	for (const l of stateListeners) l(next);
}

function backoffDelay(): number {
	// 1s, 2s, 4s, 8s, 16s, 30s (cap)
	const exp = Math.min(BASE_DELAY_MS * 2 ** attempt, MAX_DELAY_MS);
	return exp;
}

function tryConnect() {
	if (socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) {
		return;
	}

	setState('connecting');
	socket = new WebSocket(getWsUrl());

	socket.onopen = () => {
		attempt = 0;
		setState('connected');
	};

	socket.onmessage = (event) => {
		try {
			const msg: MetricsMessage = JSON.parse(event.data);
			for (const listener of listeners) {
				listener(msg);
			}
		} catch (err) {
			// Server protocol violation. Log so deploy schema drift / partial
			// frames don't surface as silent stale charts.
			console.error('ws: failed to parse metrics frame', err, event.data);
		}
	};

	socket.onclose = () => {
		socket = null;
		setState('disconnected');
		if (listeners.size === 0) return;

		const delay = backoffDelay();
		attempt += 1;
		reconnectTimer = setTimeout(tryConnect, delay);
	};

	socket.onerror = () => {
		socket?.close();
	};
}

export function subscribe(listener: Listener): () => void {
	listeners.add(listener);
	tryConnect();

	return () => {
		listeners.delete(listener);
		if (listeners.size === 0) {
			if (reconnectTimer) clearTimeout(reconnectTimer);
			reconnectTimer = null;
			attempt = 0;
			socket?.close();
			socket = null;
			setState('disconnected');
		}
	};
}

export function subscribeState(listener: StateListener): () => void {
	stateListeners.add(listener);
	listener(currentState);
	return () => stateListeners.delete(listener);
}

export function getState(): WsState {
	return currentState;
}
