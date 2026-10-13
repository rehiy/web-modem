import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const apiTarget = process.env.WEB_MODEM_DEV_API_TARGET || "http://127.0.0.1:8080";

export default defineConfig({
  base: "./",
  plugins: [react()],
  build: {
    outDir: "../webview/dist",
    emptyOutDir: true
  },
  server: {
    port: 5173,
    proxy: {
      "/api": apiTarget,
      "/events": apiTarget
    }
  }
});
