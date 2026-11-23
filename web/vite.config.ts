import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// bv serve (the Go API) listens on BV_API_PORT; the dev server proxies to it.
function apiPort(): number {
  const raw = process.env.BV_API_PORT ?? "7777";
  const port = Number(raw);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`BV_API_PORT must be an integer 1-65535, got ${JSON.stringify(raw)}`);
  }
  return port;
}

// 127.0.0.1, not localhost: bv binds IPv4 only and localhost may resolve to ::1.
const target = `http://127.0.0.1:${apiPort()}`;

export default defineConfig({
  plugins: [svelte()],
  server: {
    // strictPort: bv's origin guard only allows the dev origin :5173.
    port: 5173,
    strictPort: true,
    proxy: {
      "/api/": { target, ws: true },
      "/auth": { target },
    },
  },
  build: {
    // dist/ itself holds the committed placeholder page (index.html) that keeps
    // Go-only builds working; Vite only ever empties dist/app.
    outDir: "dist/app",
    emptyOutDir: true,
    assetsDir: "assets",
    // CSP is default-src 'self', which blocks data: URIs.
    assetsInlineLimit: 0,
    modulePreload: { polyfill: false },
  },
});