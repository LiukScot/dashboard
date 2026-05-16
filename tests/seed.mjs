// Seeds the smoke DB with the e2e user. Spawned by Playwright's webServer
// command before the dashboard binary starts.
//
// Uses the built ./user-cli-smoke binary so the seeding path exercises the
// real internal/db migrations + auth.CreateUser. Avoids re-implementing
// password hashing in JS.

import { spawn } from 'node:child_process';
import { existsSync, unlinkSync } from 'node:fs';
import path from 'node:path';

const root = path.resolve(new URL('..', import.meta.url).pathname);
const bin = path.join(root, 'user-cli-smoke');

if (!existsSync(bin)) {
	console.error(`seed: missing ${bin}; run "bun run smoke:build:go" first`);
	process.exit(1);
}

const dbPath = process.env.DB_PATH;
if (!dbPath) {
	console.error('seed: DB_PATH env var is required');
	process.exit(1);
}

// Wipe any previous WAL/SHM so tests start from a clean schema state.
for (const suffix of ['', '-wal', '-shm']) {
	const f = dbPath + suffix;
	if (existsSync(f)) unlinkSync(f);
}

const email = process.env.E2E_EMAIL || 'smoke@example.com';
const password = process.env.E2E_PASSWORD || 'Password123';

const child = spawn(bin, ['create'], {
	env: { ...process.env, DB_PATH: dbPath },
	stdio: ['pipe', 'inherit', 'inherit']
});

child.stdin.write(`${email}\n${password}\n`);
child.stdin.end();

child.on('exit', (code) => {
	if (code !== 0) {
		console.error(`seed: user-cli exited with code ${code}`);
		process.exit(code ?? 1);
	}
	console.log(`seed: created ${email} in ${dbPath}`);
});
