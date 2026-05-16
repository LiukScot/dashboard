import { expect, type APIRequestContext, type Page } from '@playwright/test';

export const e2eUser = {
	email: process.env.E2E_EMAIL || 'smoke@example.com',
	password: process.env.E2E_PASSWORD || 'Password123'
};

export async function loginUi(page: Page, password = e2eUser.password) {
	await page.context().clearCookies();
	await page.goto('/');
	// Layout renders spinner while api.session() resolves; wait for the
	// login form to mount before interacting with its inputs.
	await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible({ timeout: 30_000 });
	await page.getByLabel('Email').fill(e2eUser.email);
	await page.getByLabel('Password').fill(password);
	// Wait for the POST to land so the cookie is set and the layout can
	// observe the new session state on its next tick.
	await Promise.all([
		page.waitForResponse((r) => r.url().endsWith('/api/v1/auth/login') && r.status() === 200),
		page.getByRole('button', { name: 'Sign in' }).click()
	]);
	// Sidebar heading appears only after the layout switches to the app
	// shell (post-login). Be explicit with `Sign out` since heading text
	// can collide with the pre-login form title.
	await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
}

export async function loginApi(request: APIRequestContext, password = e2eUser.password) {
	const response = await request.post('/api/v1/auth/login', {
		data: { email: e2eUser.email, password }
	});
	if (!response.ok()) {
		const body = await response.text();
		expect(
			response.ok(),
			`expected API login to succeed for ${e2eUser.email}; status=${response.status()} body=${body}`
		).toBeTruthy();
	}
}
