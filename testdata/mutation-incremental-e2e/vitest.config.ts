import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  resolve: { alias: [{ find: /^@app\//, replacement: fileURLToPath(new URL('./src/', import.meta.url)) }] },
  test: { include: ['tests/**/*.test.ts'] },
});
