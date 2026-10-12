import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { readFileSync } from 'node:fs'

export default defineConfig({
  plugins: [react(), {
    name: 'preserve-embed-placeholder',
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'README.txt', source: placeholder })
    },
  }],
  build: {
    outDir: '../internal/webui/assets',
    emptyOutDir: true,
  },
})

const placeholder = readFileSync(new URL('../internal/webui/assets/README.txt', import.meta.url), 'utf8')
