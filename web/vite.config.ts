import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],

  build: {
    // The Go binary embeds this directory. Emptying it on every build means a
    // renamed bundle can never leave a stale sibling behind to be embedded.
    outDir: 'dist',
    emptyOutDir: true,
  },

  server: {
    port: 5173,
    // `npm run dev` gives hot reload while the Go server owns the API. In a
    // built binary there is no proxy: the same origin serves both.
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },

  test: {
    // No jsdom, deliberately: the client's tests are about pure reducers and
    // about Pixi display objects, both of which run in Node. The setup file is
    // the one exception — see the comment in it.
    setupFiles: ['./vitest.setup.ts'],
  },
})
