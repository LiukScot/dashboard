import { expect, test } from '@playwright/test';
import { loginUi } from './helpers';

test.describe('Security view', () => {
	test('loads fail2ban and logs sections', async ({ page }) => {
		await loginUi(page);

		await page.getByRole('link', { name: /Security/i }).click();
		await expect(page).toHaveURL(/\/security/);
		await expect(page.getByRole('heading', { name: /Security & Alerts/i })).toBeVisible();
	});
});
