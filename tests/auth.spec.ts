import { expect, test } from '@playwright/test';
import { e2eUser, loginUi } from './helpers';

test('healthz endpoint reports ok', async ({ request }) => {
	const res = await request.get('/healthz');
	expect(res.ok()).toBeTruthy();
	const body = await res.json();
	expect(body.status).toBe('ok');
	expect(typeof body.uptime).toBe('number');
});

test('unauthenticated session endpoint returns authenticated=false', async ({ request }) => {
	const res = await request.get('/api/v1/auth/session');
	expect(res.ok()).toBeTruthy();
	const body = await res.json();
	expect(body.authenticated).toBe(false);
});

test('login then logout flow', async ({ page }) => {
	await loginUi(page);

	// Evaluate from inside the page so the request carries the same
	// cookie jar as the active session. page.request uses a different
	// context.
	const body = await page.evaluate(async () => {
		const r = await fetch('/api/v1/auth/session', { credentials: 'include' });
		return r.json();
	});
	expect(body.authenticated).toBe(true);

	// Click "Sign out" in sidebar.
	await page.getByRole('button', { name: 'Sign out' }).click();

	// Login form re-appears.
	await expect(page.getByPlaceholder('Email')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();
});

test('login rejects wrong password', async ({ page }) => {
	await page.context().clearCookies();
	await page.goto('/');
	await page.getByPlaceholder('Email').fill(e2eUser.email);
	await page.getByPlaceholder('Password').fill('not-the-password');
	await page.getByRole('button', { name: 'Sign in' }).click();

	// Inline error (role=alert) surfaces invalid credentials.
	await expect(page.getByRole('alert')).toBeVisible();
});
