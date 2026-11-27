import tailwindcss from '@tailwindcss/vite';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';
import { playwright } from '@vitest/browser-playwright';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';

export default defineConfig(({ mode }) => {
	const rootEnv = loadEnv(mode, '..', '');
	const uiEnv = loadEnv(mode, '.', '');
	const env = { ...rootEnv, ...uiEnv };
	const rawHost = (env.HOST || 'localhost').trim();
	const proxyHost = rawHost === '' || rawHost === '0.0.0.0' || rawHost === '::' ? 'localhost' : rawHost;
	const proxyPort = (env.PORT || '3000').trim();
	const backendTarget = (env.MICROBASE_URL || `http://${proxyHost}:${proxyPort}`).replace(/\/+$/, '');

	return {
		plugins: [
			tailwindcss(),
			sveltekit({
				compilerOptions: {
					runes: ({ filename }) =>
						filename.split(/[/\\]/).includes('node_modules') ? undefined : true
				},
				adapter: adapter({
					pages: 'build',
					assets: 'build',
					fallback: 'index.html'
				}),
				paths: {
					base: '/_'
				}
			})
		],
		server: {
			proxy: {
				'/api': {
					target: backendTarget,
					changeOrigin: true,
					ws: true
				},
				'/health': {
					target: backendTarget,
					changeOrigin: true
				}
			}
		},
		test: {
			expect: { requireAssertions: true },
			projects: [
				{
					extends: './vite.config.ts',
					test: {
						name: 'client',
						browser: {
							enabled: true,
							provider: playwright(),
							instances: [{ browser: 'chromium', headless: true }]
						},
						include: ['src/**/*.svelte.{test,spec}.{js,ts}'],
						exclude: ['src/lib/server/**']
					}
				},
				{
					extends: './vite.config.ts',
					test: {
						name: 'server',
						environment: 'node',
						include: ['src/**/*.{test,spec}.{js,ts}'],
						exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
					}
				}
			]
		}
	};
});
