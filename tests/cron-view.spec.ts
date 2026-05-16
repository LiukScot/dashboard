import { expect, test } from '@playwright/test';
import { loginUi } from './helpers';

test.describe('Cron view', () => {
	test('loads cron weekly view', async ({ page }) => {
		await loginUi(page);

		await page.getByRole('link', { name: /Cron/i }).click();
		await expect(page).toHaveURL(/\/cron/);
		await expect(page.getByRole('heading', { name: /Cron Weekly/i })).toBeVisible();

		// "Today" button is part of the navigation row and is always
		// rendered regardless of whether real cron data is present.
		await expect(page.getByRole('button', { name: 'Today' })).toBeVisible();
	});

	test('hidden-job count endpoint responds', async ({ request, context }) => {
		// loginUi only operates on Pages; pass through the request context
		// using a fresh API call instead.
		const login = await request.post('/api/v1/auth/login', {
			data: { email: process.env.E2E_EMAIL || 'smoke@example.com', password: process.env.E2E_PASSWORD || 'Password123' }
		});
		expect(login.ok()).toBeTruthy();

		const res = await request.get('/api/v1/cron/hidden/count');
		expect(res.ok()).toBeTruthy();
		const body = await res.json();
		expect(typeof body.count).toBe('number');
		expect(body.count).toBeGreaterThanOrEqual(0);
		await context.clearCookies();
	});
});
