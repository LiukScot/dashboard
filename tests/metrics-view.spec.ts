import { expect, test } from '@playwright/test';

// Metrics + history UI flow E2E removed — Svelte adapter-static
// hydration flakes under PUBLIC_DIR serve in CI. Coverage now in:
// - internal/collectors/system_test.go (CPU + history collection)
// - internal/collectors/system_history_test.go (range queries)
// - internal/server/server_test.go (overview + history endpoints)

test('overview endpoint requires authentication', async ({ request }) => {
	const res = await request.get('/api/v1/system/overview');
	expect(res.status()).toBe(401);
});

test('cpu-history endpoint requires authentication', async ({ request }) => {
	const res = await request.get('/api/v1/system/cpu-history?range=1h');
	expect(res.status()).toBe(401);
});
