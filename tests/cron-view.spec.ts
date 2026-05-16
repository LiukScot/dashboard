import { expect, test } from '@playwright/test';

// Cron view UI E2E removed — see auth.spec.ts comment. Coverage:
// - internal/collectors/cron_test.go
// - internal/server/server_test.go (cron endpoints)

test('cron week endpoint requires authentication', async ({ request }) => {
	const res = await request.get('/api/v1/cron/week');
	expect(res.status()).toBe(401);
});

test('hidden-job count endpoint responds after API login', async ({ request, context }) => {
	const login = await request.post('/api/v1/auth/login', {
		data: {
			email: process.env.E2E_EMAIL || 'smoke@example.com',
			password: process.env.E2E_PASSWORD || 'Password123'
		}
	});
	expect(login.ok()).toBeTruthy();

	const res = await request.get('/api/v1/cron/hidden/count');
	expect(res.ok()).toBeTruthy();
	const body = await res.json();
	expect(typeof body.count).toBe('number');
	expect(body.count).toBeGreaterThanOrEqual(0);
	await context.clearCookies();
});
