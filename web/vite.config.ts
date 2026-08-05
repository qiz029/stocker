/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: {
    // core/ lives outside web/, so bare "react" imports from there would
    // walk up past the repo and find an unrelated node_modules. Pin them.
    alias: {
      react: new URL("./node_modules/react", import.meta.url).pathname,
      "react-dom": new URL("./node_modules/react-dom", import.meta.url).pathname,
    },
  },
  server: {
    proxy: { "/api": "http://localhost:8080" },
    // web/src shims re-export from ../core (shared with the RN app).
    fs: { allow: [".."] },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    globals: true,
  },
});
