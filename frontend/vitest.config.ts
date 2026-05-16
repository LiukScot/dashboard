import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import path from 'node:path';

// Vitest config keeps the Svelte plugin so .svelte imports resolve, but
// skips Tailwind + the SvelteKit plugin: neither is needed for component
// unit tests and SvelteKit's plugin breaks under vitest (no app context).
export default defineConfig({
	plugins: [svelte({ hot: false })],
	resolve: {
		alias: {
			$lib: path.resolve(__dirname, 'src/lib')
		},
		conditions: ['browser']
	},
	test: {
		environment: 'jsdom',
		globals: true,
		setupFiles: ['./src/test-setup.ts'],
		include: ['src/**/*.{test,spec}.{js,ts}']
	}
});
