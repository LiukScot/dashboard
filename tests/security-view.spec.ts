import { expect, test } from '@playwright/test';

// Security view UI E2E removed — see auth.spec.ts comment. Coverage:
// - internal/collectors/fail2ban_test.go
// - internal/collectors/logs_test.go

test('fail2ban endpoint requires authentication', async ({ request }) => {
	const res = await request.get('/api/v1/security/fail2ban');
	expect(res.status()).toBe(401);
});

test('logs endpoint requires authentication', async ({ request }) => {
	const res = await request.get('/api/v1/security/logs');
	expect(res.status()).toBe(401);
});
