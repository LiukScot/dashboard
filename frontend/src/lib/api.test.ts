import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { api } from './api';

// Capture references so we can restore between tests; jsdom doesn't ship a
// fetch implementation by default, so each test overrides it explicitly.
const originalFetch = globalThis.fetch;

describe('api client', () => {
	beforeEach(() => {
		// Reset between tests; vi.fn re-creates the spy.
		globalThis.fetch = vi.fn();
	});

	afterEach(() => {
		globalThis.fetch = originalFetch;
		vi.restoreAllMocks();
	});

	it('login posts JSON body and resolves with parsed response', async () => {
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: true,
			headers: new Headers({ 'content-type': 'application/json' }),
			json: async () => ({ status: 'ok' })
		});

		await api.login('alice@example.com', 'pw');

		expect(mock).toHaveBeenCalledTimes(1);
		const [url, init] = mock.mock.calls[0];
		expect(url).toBe('/api/v1/auth/login');
		expect(init.method).toBe('POST');
		expect(init.credentials).toBe('include');
		expect(JSON.parse(init.body)).toEqual({ email: 'alice@example.com', password: 'pw' });
	});

	it('throws with server error message on non-2xx JSON response', async () => {
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: false,
			statusText: 'Unauthorized',
			headers: new Headers({ 'content-type': 'application/json' }),
			json: async () => ({ error: 'invalid credentials' })
		});

		await expect(api.login('a', 'b')).rejects.toThrow('invalid credentials');
	});

	it('falls back to statusText when body has no error field', async () => {
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: false,
			statusText: 'Not Found',
			headers: new Headers({ 'content-type': 'application/json' }),
			json: async () => ({})
		});

		await expect(api.session()).rejects.toThrow('Not Found');
	});

	it('rejects non-JSON responses to prevent silent HTML-as-data', async () => {
		// A common dev mistake: SPA fallback HTML returned for an unknown
		// /api/* route. The client must surface this, not pretend the
		// HTML is JSON.
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: true,
			headers: new Headers({ 'content-type': 'text/html' }),
			json: async () => ({})
		});

		await expect(api.session()).rejects.toThrow(/Expected JSON/);
	});

	it('session returns only the authenticated flag (no user PII)', async () => {
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: true,
			headers: new Headers({ 'content-type': 'application/json' }),
			json: async () => ({ authenticated: true })
		});

		const session = await api.session();

		const [url] = mock.mock.calls[0];
		expect(url).toBe('/api/v1/auth/session');
		expect(session).toEqual({ authenticated: true });
		expect('user' in session).toBe(false);
	});

	it('me returns the user object from the auth-gated endpoint', async () => {
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: true,
			headers: new Headers({ 'content-type': 'application/json' }),
			json: async () => ({ id: 1, email: 'alice@example.com' })
		});

		const user = await api.me();

		const [url] = mock.mock.calls[0];
		expect(url).toBe('/api/v1/auth/me');
		expect(user).toEqual({ id: 1, email: 'alice@example.com' });
	});

	it('systemHistory passes range as query string', async () => {
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: true,
			headers: new Headers({ 'content-type': 'application/json' }),
			json: async () => []
		});

		await api.systemHistory('24h');

		const [url] = mock.mock.calls[0];
		expect(url).toBe('/api/v1/system/history?range=24h');
	});

	it('logs builds query string only for non-default filters', async () => {
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: true,
			headers: new Headers({ 'content-type': 'application/json' }),
			json: async () => []
		});

		await api.logs('sshd.service', 3, 50);

		const [url] = mock.mock.calls[0];
		expect(url).toMatch(/^\/api\/v1\/security\/logs\?/);
		expect(url).toContain('unit=sshd.service');
		expect(url).toContain('priority=3');
		expect(url).toContain('limit=50');
	});

	it('hideCronJob URL-encodes the fingerprint', async () => {
		const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
		mock.mockResolvedValueOnce({
			ok: true,
			headers: new Headers({ 'content-type': 'application/json' }),
			json: async () => ({ status: 'ok' })
		});

		await api.hideCronJob('cron/cd:5#0+!');

		const [url, init] = mock.mock.calls[0];
		expect(init.method).toBe('POST');
		// Slashes, colons, and ! must all be encoded.
		expect(url).toBe('/api/v1/cron/jobs/cron%2Fcd%3A5%230%2B!/hide');
	});
});
