import path from "node:path";
import http from "node:http";
import type { IncomingMessage, ServerResponse } from "node:http";
import type { ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv, type Plugin } from "vite";
import { ingressHostToDashboardPath } from "./src/lib/ingress-routing";

const API_TARGET = "http://127.0.0.1:8790";
const GATEWAY_TARGET = "http://127.0.0.1:8787";
const INGRESS_COOKIE = "cellp_ingress";

function parseIngressHost(req: IncomingMessage): string | null {
  const cookie = req.headers.cookie ?? "";
  const m = cookie.match(new RegExp(`${INGRESS_COOKIE}=([^;]+)`));
  if (!m) return null;
  try {
    return decodeURIComponent(m[1]);
  } catch {
    return null;
  }
}

/** Dashboard SPA routes — do not proxy to gateway. */
function shouldSkipIngressProxy(url: string): boolean {
  const p = url.split("?")[0] ?? url;
  if (
    p.startsWith("/@") ||
    p.startsWith("/node_modules") ||
    p.startsWith("/src") ||
    p.startsWith("/v1") ||
    p.startsWith("/metrics") ||
    p.startsWith("/__gateway") ||
    p === "/" ||
    p.startsWith("/projects") ||
    p.startsWith("/platform") ||
    p.startsWith("/deployments") ||
    p.startsWith("/settings") ||
    p.startsWith("/embed")
  ) {
    return true;
  }
  if (/\.(js|css|map|ico|svg|png|webp|woff2?|ttf|html)(\?|$)/i.test(p)) {
    return true;
  }
  return false;
}

function ingressGatewayRedirectPlugin(): Plugin {
  return {
    name: "cellp-ingress-gateway-redirect",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const raw = req.url ?? "";
        if (!raw.startsWith("/__gateway")) {
          next();
          return;
        }
        try {
          const u = new URL(raw, "http://127.0.0.1");
          const host = u.searchParams.get("__cellp_host");
          if (host) {
            const dest = ingressHostToDashboardPath(host);
            if (dest) {
              res.statusCode = 302;
              res.setHeader("Location", dest);
              res.end();
              return;
            }
          }
        } catch {
          /* fall through to gateway proxy */
        }
        next();
      });
    },
  };
}

function ingressCookieProxyPlugin(): Plugin {
  return {
    name: "cellp-ingress-cookie-proxy",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = req.url ?? "/";
        const host = parseIngressHost(req);
        if (!host || shouldSkipIngressProxy(url)) {
          next();
          return;
        }

        const proxyReq = http.request(
          {
            hostname: "127.0.0.1",
            port: 8787,
            path: url,
            method: req.method,
            headers: {
              ...req.headers,
              host,
            },
          },
          (proxyRes) => {
            res.writeHead(proxyRes.statusCode ?? 502, proxyRes.headers);
            proxyRes.pipe(res);
          },
        );
        proxyReq.on("error", () => {
          if (!res.headersSent) {
            res.statusCode = 502;
            res.end("gateway proxy error");
          }
        });
        req.pipe(proxyReq);
      });
    },
  };
}

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
      proxy.on("proxyRes", (proxyRes, req, res) => {
        try {
          const url = new URL(req.url ?? "/", "http://127.0.0.1");
          const host = url.searchParams.get("__cellp_host");
          if (host && res instanceof ServerResponse && !res.headersSent) {
            res.setHeader(
              "Set-Cookie",
              `${INGRESS_COOKIE}=${encodeURIComponent(host)}; Path=/; SameSite=Lax; Max-Age=3600`,
            );
          }
        } catch {
          /* ignore */
        }
        void proxyRes;
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
    plugins: [react(), ingressGatewayRedirectPlugin(), ingressCookieProxyPlugin()],
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
