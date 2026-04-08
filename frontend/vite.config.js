import { defineConfig } from 'vite'

export default defineConfig({
  build: {
    outDir: '../internal/web/static/dist',
    emptyOutDir: true,
    lib: {
      entry: 'src/editor.js',
      name: 'prd2wiki',
      fileName: 'editor',
      formats: ['iife']
    },
    rollupOptions: {
      output: {
        assetFileNames: '[name][extname]'
      }
    }
  }
})
