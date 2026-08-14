/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Absolute origin used to expand __SITE_URL__ in the static HTML pages
// (og:url, og:image, canonical). Set VITE_SITE_URL when building for prod.
declare const process: { env: Record<string, string | undefined> };
const siteUrl = (process.env.VITE_SITE_URL ?? "https://stocker.example.com").replace(/\/+$/, "");

// Static marketing pages; keys become output paths (scenarios/x → dist/scenarios/x.html).
const publicPages = [
  "landing.html",
  "landing-en.html",
  "scenarios/dotcom-2000.html",
  "scenarios/black-monday-1987.html",
  "scenarios/nifty-fifty-1972.html",
  "scenarios/gfc-2008.html",
  "scenarios-en/dotcom-2000.html",
  "scenarios-en/black-monday-1987.html",
  "scenarios-en/nifty-fifty-1972.html",
  "scenarios-en/gfc-2008.html",
];

export default defineConfig({
  plugins: [
    react(),
    { name: "site-url", transformIndexHtml: (html) => html.replaceAll("__SITE_URL__", siteUrl) },
    {
      name: "sitemap",
      generateBundle() {
        const urls = ["", ...publicPages]
          .map((p) => `  <url><loc>${siteUrl}/${p}</loc></url>`)
          .join("\n");
        this.emitFile({
          type: "asset",
          fileName: "sitemap.xml",
          source: `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls}\n</urlset>\n`,
        });
      },
    },
  ],
  build: {
    rollupOptions: {
      input: Object.fromEntries(
        ["index.html", ...publicPages].map((p) => [
          p.replace(/\.html$/, ""),
          new URL(`./${p}`, import.meta.url).pathname,
        ]),
      ),
    },
  },
  resolve: {
    // core/ lives outside web/, so bare "react" imports from there would
    // walk up past the repo and find an unrelated node_modules. Pin them.
    alias: {
      react: new URL("./node_modules/react", import.meta.url).pathname,
      "react-dom": new URL("./node_modules/react-dom", import.meta.url).pathname,
    },
  },
  server: {
    // /share/* is server-rendered by the Go API (public battle-report pages).
    proxy: { "/api": "http://localhost:8080", "/share": "http://localhost:8080" },
    // web/src shims re-export from ../core (shared with the RN app).
    fs: { allow: [".."] },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    globals: true,
  },
});
