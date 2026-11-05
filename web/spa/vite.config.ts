/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The SPA is served by the Go binary at / (web/spa/spa.go embeds dist/). Its hashed files go
// under /static/ because /assets/ is the engine's own (app.css, fonts, logo).
// BREVIS_API points the dev server at another API (a second one on its own port).
const api = process.env.BREVIS_API ?? "http://localhost:8080";

export default defineConfig({
  base: "/",
  plugins: [
    react(),
    {
      // In production the Go binary swaps the placeholder for the stylesheet's
      // versioned URL (spa.go); in dev the server's own sheet is proxied.
      name: "brevis-app-css",
      apply: "serve",
      transformIndexHtml: (html) => html.replace("<!--brevis:app-css-->", '<link rel="stylesheet" href="/assets/app.css" />'),
    },
  ],
  // Unit tests (npm run test). jsdom because the logic under test touches `document`,
  // `localStorage` and React; e2e/ belongs to Playwright and must not be collected here.
  test: { environment: "jsdom", setupFiles: ["src/test/setup.ts"], include: ["src/**/*.test.{ts,tsx}"], restoreMocks: true },
  build: { outDir: "dist", assetsDir: "static", emptyOutDir: true, sourcemap: false },
  // The graph (React Flow) is only imported by a lazy chunk. Pre-bundled up front it is
  // never "discovered" in the middle of a session: a late discovery makes the dev server
  // re-optimize, and every open tab then fails with 504 "Outdated Optimize Dep" and
  // "Failed to fetch dynamically imported module" until it is reloaded.
  optimizeDeps: { include: ["@xyflow/react"] },
  server: {
    // The logo marks live with the server's components (web/components/logos).
    fs: { allow: [".."] },
    // `make up` serves the API on 8080; the dev server proxies to it, so the
    // browser sees one origin and the session cookie works without CORS.
    proxy: {
      "/api": api,
      "/health": api,
      "/assets": api,
    },
  },
});
