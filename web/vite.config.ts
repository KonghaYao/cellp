import path from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

const API_TARGET = "http://127.0.0.1:8790";
const GATEWAY_TARGET = "http://127.0.0.1:8787";

const devProxy = {
  "/v1": { target: API_TARGET, changeOrigin: true },
  "/metrics": { target: API_TARGET, changeOrigin: true },
  "/__gateway": {
    target: GATEWAY_TARGET,
    changeOrigin: true,
    rewrite: (p: string) => p.replace(/^\/__gateway/, ""),
  },
};

export default defineConfig(({ mode }) => {
  loadEnv(mode, process.cwd(), "");
  return {
  plugins: [react()],
  base: process.env.VITE_BASE ?? "./",
  build: {
    assetsDir: ".",
    rollupOptions: {
      output: {
        entryFileNames: "dashboard.js",
        assetFileNames: "dashboard.[ext]",
      },
    },
  },
  server: {
    host: "127.0.0.1",
    port: Number(process.env.DASHBOARD_PORT) || 5190,
    proxy: devProxy,
  },
  preview: {
    host: "127.0.0.1",
    port: Number(process.env.DASHBOARD_PREVIEW_PORT) || 4173,
    proxy: devProxy,
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  };
});
