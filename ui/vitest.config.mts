import path from 'node:path';
import {defineConfig} from 'vitest/config';

export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, 'src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    testTimeout: 10_000,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}', 'scripts/**/*.test.mjs'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/**/*.test.{ts,tsx}',
        'src/test/**',
        'src/lib/api/proto/**',
        'src/dev/**',
        'src/app/layout.tsx',
        'src/app/(app)/layout.tsx',
        'src/app/(app)/share/layout.tsx',
        'src/lib/providers/QueryProvider.tsx',
      ],
      // 90 across the board: every new feature ships with the tests that keep
      // it there (see ui/docs/testing.md). Raising these is fine; lowering one
      // to land a change is not — write the missing test instead.
      thresholds: {
        lines: 90,
        functions: 90,
        statements: 90,
        branches: 90,
      },
    },
  },
});
