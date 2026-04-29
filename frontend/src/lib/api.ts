const BASE = '';

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		credentials: 'include',
		headers: { 'Content-Type': 'application/json' },
		...opts
	});

	if (!res.ok) {
		const body = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(body.error || res.statusText);
	}

	const contentType = res.headers.get('content-type') ?? '';
	if (!contentType.includes('application/json')) {
		throw new Error(`Expected JSON from ${path}, got ${contentType || 'unknown content type'}`);
	}

	return res.json();
}

export const api = {
	login: (email: string, password: string) =>
		request('/api/v1/auth/login', {
			method: 'POST',
			body: JSON.stringify({ email, password })
		}),
	logout: () => request('/api/v1/auth/logout', { method: 'POST' }),
	session: () => request<SessionResponse>('/api/v1/auth/session'),
	me: () => request<{ id: number; email: string }>('/api/v1/auth/me'),

	systemOverview: () => request<SystemMetrics>('/api/v1/system/overview'),
	cpuHistory: () => request<SystemMetrics[]>('/api/v1/system/cpu-history'),
	network: () => request<NetworkMetrics[]>('/api/v1/system/network'),

	dockerContainers: () => request<Container[]>('/api/v1/docker/containers'),

	fail2ban: () => request<Fail2BanStatus | null>('/api/v1/security/fail2ban'),
	fail2banBans: (limit = 50) => request<BanEvent[]>(`/api/v1/security/fail2ban/bans?limit=${limit}`),
	logs: (unit = '', priority = -1, limit = 100) => {
		const params = new URLSearchParams();
		if (unit) params.set('unit', unit);
		if (priority >= 0) params.set('priority', String(priority));
		params.set('limit', String(limit));
		return request<LogEntry[]>(`/api/v1/security/logs?${params}`);
	},
	cronWeek: (start: string) => request<CronWeek>(`/api/v1/cron/week?start=${start}`),
	hideCronJob: (fingerprint: string) =>
		request<{ status: string }>(`/api/v1/cron/jobs/${encodeURIComponent(fingerprint)}/hide`, {
			method: 'POST'
		}),
	resetHiddenCronJobs: () => request<{ status: string }>('/api/v1/cron/hidden', { method: 'DELETE' }),
	hiddenCronJobCount: () => request<{ count: number }>('/api/v1/cron/hidden/count')
};

export interface SystemMetrics {
	hostname: string;
	uptime: number;
	loadAvg: [number, number, number];
	cpuPercent: number;
	cpuCores: number;
	memTotal: number;
	memUsed: number;
	memPercent: number;
	swapTotal: number;
	swapUsed: number;
	diskTotal: number;
	diskUsed: number;
	diskPercent: number;
	timestamp: string;
}

export interface NetworkMetrics {
	interface: string;
	rxBytes: number;
	txBytes: number;
	rxRate: number;
	txRate: number;
}

export interface Container {
	id: string;
	name: string;
	image: string;
	state: string;
	status: string;
	created: number;
	ports: { ip: string; privatePort: number; publicPort: number; type: string }[];
}

export interface ContainerStats {
	id: string;
	name: string;
	cpuPercent: number;
	memUsage: number;
	memLimit: number;
	memPercent: number;
	netRx: number;
	netTx: number;
}

export interface Fail2BanStatus {
	jails: JailStatus[];
	totalBans: number;
	totalJails: number;
}

export interface JailStatus {
	name: string;
	bannedIPs: string[];
	banCount: number;
	totalBans: number;
	totalFails: number;
}

export interface BanEvent {
	timestamp: string;
	jail: string;
	ip: string;
	action: string;
}

export interface LogEntry {
	timestamp: string;
	unit: string;
	message: string;
	priority: number;
	priorityLabel: string;
	hostname: string;
	pid: string;
}

export interface SessionResponse {
	authenticated: boolean;
	user?: { id: number; email: string };
}

export interface CronWeek {
	start: string;
	end: string;
	days: string[];
	timezone: string;
	historyCoverage: 'none' | 'partial' | 'good';
	hiddenJobCount: number;
	jobs: CronJob[];
	occurrences: CronOccurrence[];
	history: CronHistoryItem[];
	warnings: string[];
}

export interface CronJob {
	fingerprint: string;
	source: string;
	line: number;
	schedule: string;
	user: string;
	command: string;
}

export interface CronOccurrence {
	id: string;
	jobId: string;
	scheduledAt: string;
	dayKey: string;
	minutesOfDay: number;
	displayTime: string;
	status: 'planned' | 'scheduled' | 'observed' | 'failed';
	source: string;
	user: string;
	command: string;
}

export interface CronHistoryItem {
	jobId: string;
	scheduledAt: string;
	observedAt: string;
	status: string;
	source: string;
	message: string;
}
