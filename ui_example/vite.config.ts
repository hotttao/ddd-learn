import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import { defineConfig } from "vite";

const analyze = process.env.ANALYZE === "true";

export default defineConfig({
  plugins: [
    react(),
    analyze && visualizer({ filename: "stats.html", sourcemap: true }),
  ],
  // Emit web workers as ES modules so code-splitting works
  // (Rollup rejects the default iife format for code-split builds).
  worker: {
    format: "es",
  },
  server: {
    // Listen on the host LAN address so other devices can open the UI.
    host: "192.168.2.41",
    port: 5173,
    // Browser requests use the same-origin /kratos path. Vite forwards them
    // to the Traefik entry point; Traefik then routes them to Kratos.
    proxy: {
      "/kratos": {
        target: "http://192.168.2.41:8080",
        changeOrigin: false,
      },
    },
  },
  build: {
    sourcemap: analyze,
    chunkSizeWarningLimit: 1024,
  },
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },
});
