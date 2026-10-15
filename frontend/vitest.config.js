// Tests unitarios puros (node): NO cargan el plugin de SvelteKit.
import { defineConfig } from 'vitest/config';

export default defineConfig({
	test: {
		environment: 'node',
		include: ['src/**/*.test.js']
	}
});
