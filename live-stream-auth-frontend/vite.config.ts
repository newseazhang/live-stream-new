import { defineConfig, loadEnv } from 'vite';
import vinext from 'vinext';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'LIVE_STREAM_');
  const target = process.env.LIVE_STREAM_BACKEND_URL || env.LIVE_STREAM_BACKEND_URL || 'http://127.0.0.1:19000';

  return {
    plugins: [vinext()],
    server: {
      host: '127.0.0.1',
      port: 13000,
      strictPort: true,
      proxy: {
        '/api': { target, changeOrigin: true, ws: true },
      },
    },
  };
});
