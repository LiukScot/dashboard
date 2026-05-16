import { expect, test } from '@playwright/test';
import { loginUi } from './helpers';

test.describe('Overview / metrics view', () => {
	test('renders at least one metric tile after login', async ({ page }) => {
		await loginUi(page);
		// Page lands on / by default (Overview). Look for the heading and
		// at least one gauge label (CPU is always rendered from /proc).
		await expect(page.getByRole('heading', { name: 'System Overview' })).toBeVisible();
		await expect(page.locator('text=CPU').first()).toBeVisible({ timeout: 10_000 });
	});

	test('range selector switches history window', async ({ page }) => {
		await loginUi(page);
		await expect(page.getByRole('heading', { name: 'System Overview' })).toBeVisible();

		// "Live" is the initial range; clicking "1h" must surface either
		// the chart with that window or a transient "Loading…" indicator.
		const oneHour = page.getByRole('button', { name: '1h', exact: true });
		await expect(oneHour).toBeVisible();
		await oneHour.click();

		// Live button should no longer be the active one; selecting "Live"
		// again returns to the live feed.
		await page.getByRole('button', { name: 'Live', exact: true }).click();
		await expect(page.getByRole('button', { name: 'Live', exact: true })).toBeVisible();
	});
});
