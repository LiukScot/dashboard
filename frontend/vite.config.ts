import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	build: {
		rollupOptions: {
			output: {
				// Isolate ECharts (~552 KB) into its own chunk so it stays out of
				// the main route bundle and only loads when a chart is rendered.
				manualChunks(id) {
					if (id.includes('node_modules/echarts') || id.includes('node_modules/zrender')) {
						return 'echarts';
					}
				}
			}
		}
	},
	server: {
		proxy: {
			'/api': 'http://localhost:4200',
			'/ws': {
				target: 'ws://localhost:4200',
				ws: true
			}
		}
	}
});
