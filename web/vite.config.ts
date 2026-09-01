import path from "node:path";
import type { ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

const API_TARGET = "http://127.0.0.1:8790";
const GATEWAY_TARGET = "http://127.0.0.1:8787";

function gatewayProxy(): ProxyOptions {
  return {
    target: GATEWAY_TARGET,
    changeOrigin: false,
    configure: (proxy) => {
      proxy.on("proxyReq", (proxyReq, req) => {
        try {
          const url = new URL(req.url ?? "/", "http://127.0.0.1");
          const host = url.searchParams.get("__cellp_host");
          if (host) {
            proxyReq.setHeader("Host", host);
            url.searchParams.delete("__cellp_host");
            const next = url.pathname + url.search;
            proxyReq.path = next || "/";
          }
        } catch {
          /* ignore */
        }
      });
    },
    rewrite: (p) => p.replace(/^\/__gateway/, "") || "/",
  };
}

const devProxy = {
  "/v1": { target: API_TARGET, changeOrigin: true },
  "/metrics": { target: API_TARGET, changeOrigin: true },
  "/__gateway": gatewayProxy(),
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
