import type { SystemMetrics, NetworkMetrics, ContainerStats } from './api';

export interface MetricsMessage {
	type: 'metrics';
	system?: SystemMetrics;
	network?: NetworkMetrics[];
	docker?: ContainerStats[];
}

type Listener = (msg: MetricsMessage) => void;

let socket: WebSocket | null = null;
let listeners: Set<Listener> = new Set();
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

function getWsUrl(): string {
	const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
	return `${proto}//${location.host}/ws`;
}

function tryConnect() {
	if (socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) {
		return;
	}

	socket = new WebSocket(getWsUrl());

	socket.onmessage = (event) => {
		try {
			const msg: MetricsMessage = JSON.parse(event.data);
			for (const listener of listeners) {
				listener(msg);
			}
		} catch {
			// ignore parse errors
		}
	};

	socket.onclose = () => {
		socket = null;
		if (listeners.size > 0) {
			reconnectTimer = setTimeout(tryConnect, 3000);
		}
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
			socket?.close();
			socket = null;
		}
	};
}
