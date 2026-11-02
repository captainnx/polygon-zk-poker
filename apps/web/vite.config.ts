import { defineConfig } from 'vite';
export default defineConfig({server:{proxy:{'/api':process.env.API_URL ?? 'http://127.0.0.1:8080','/healthz':process.env.API_URL ?? 'http://127.0.0.1:8080','/ws':{target:process.env.API_URL ?? 'http://127.0.0.1:8080',ws:true,changeOrigin:true}}}});
