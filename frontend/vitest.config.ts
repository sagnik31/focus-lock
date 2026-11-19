import { defineConfig } from 'vitest/config';

// Unit tests for pure frontend logic. No DOM: components that need the Wails
// runtime are mocked in the test files that import them.
export default defineConfig({
    test: {
        include: ['src/**/*.test.ts'],
        environment: 'node',
    },
});
