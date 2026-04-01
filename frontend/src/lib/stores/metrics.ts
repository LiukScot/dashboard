import { subscribe, type MetricsMessage } from '../ws';
import type { SystemMetrics, NetworkMetrics, ContainerStats } from '../api';

// Svelte 5 runes-compatible reactive state
let _system = $state<SystemMetrics | null>(null);
let _network = $state<NetworkMetrics[]>([]);
let _docker = $state<ContainerStats[]>([]);
let _cpuHistory = $state<{ time: string; value: number }[]>([]);
let _connected = $state(false);

let unsubscribe: (() => void) | null = null;

export function startMetrics() {
	if (unsubscribe) return;

	unsubscribe = subscribe((msg: MetricsMessage) => {
		_connected = true;

		if (msg.system) {
			_system = msg.system;
			_cpuHistory = [
				..._cpuHistory.slice(-119),
				{
					time: new Date(msg.system.timestamp).toLocaleTimeString(),
					value: msg.system.cpuPercent
				}
			];
		}
		if (msg.network) _network = msg.network;
		if (msg.docker) _docker = msg.docker;
	});
}

export function stopMetrics() {
	unsubscribe?.();
	unsubscribe = null;
	_connected = false;
}

export function getSystem() { return _system; }
export function getNetwork() { return _network; }
export function getDocker() { return _docker; }
export function getCpuHistory() { return _cpuHistory; }
export function isConnected() { return _connected; }
