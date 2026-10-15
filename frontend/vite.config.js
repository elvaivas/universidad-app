import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		proxy: {
			// en dev, el navegador habla con vite (5173) y este reenvía al Go (8080)
			'/api': 'http://localhost:8080',
			'/healthz': 'http://localhost:8080',
			'/ws': { target: 'http://localhost:8080', ws: true } // chat en tiempo real
		}
	}
});
