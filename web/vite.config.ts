import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Dev backend target. Override with VITE_DEV_API_TARGET if the backend runs
// on another host (e.g. `VITE_DEV_API_TARGET=http://192.168.1.10:8080 npm run dev`).
const devApiTarget = process.env.VITE_DEV_API_TARGET || 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: devApiTarget,
        ws: true
      }
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true
  }
})
