/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// base: '' keeps asset URLs relative so the SPA works when embedded in the Go
// binary and served from any origin (LAN IP or Tailscale hostname).
export default defineConfig({
  base: '',
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    globals: false,
  },
})
