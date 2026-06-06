import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { defineConfig } from '@playwright/test';

const smokePort = Number(process.env.SMOKE_PORT || 4200);
const smokeDbDir = fs.mkdtempSync(path.join(os.tmpdir(), 'dashboard-playwright-'));
const smokeDbPath = path.join(smokeDbDir, 'smoke-dashboard.sqlite');

const externalBaseURL = process.env.PLAYWRIGHT_BASE_URL;
const quote = (s: string) => JSON.stringify(s);

// Build steps are kept outside webServer.command so a failed build fails
// fast (and once) rather than restarting the whole web-server loop.
// Playwright runs `command` as a shell pipeline; chaining build → seed →
// serve here keeps the test runner in charge of teardown.
const webServerCommand = [
	'bun run smoke:build:fe',
	'bun run smoke:build:go',
	`DB_PATH=${quote(smokeDbPath)} bun run smoke:seed`,
	`PORT=${smokePort} HOST=127.0.0.1 ` +
		`DB_PATH=${quote(smokeDbPath)} ` +
		`PUBLIC_DIR=${quote(path.resolve('frontend/build'))} ` +
		`PROC_PATH=/proc LOG_PATH=/tmp CRON_PATHS=/etc/crontab ` +
		`ALLOWED_ORIGINS=http://127.0.0.1:${smokePort} ` +
		`COOKIE_SECURE=false ` +
		`bun run smoke:serve`
].join(' && ');

export default defineConfig({
	testDir: './tests',
	testMatch: /.*\.spec\.ts$/,
	timeout: 60_000,
	fullyParallel: false,
	workers: 1,
	retries: 0,
	use: {
		baseURL: externalBaseURL || `http://127.0.0.1:${smokePort}`,
		headless: true,
		trace: 'retain-on-failure'
	},
	webServer: externalBaseURL
		? undefined
		: {
				command: webServerCommand,
				port: smokePort,
				reuseExistingServer: false,
				timeout: 180_000,
				stdout: 'pipe',
				stderr: 'pipe'
			}
});
