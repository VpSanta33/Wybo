import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/robots.txt": { target: "http://127.0.0.1:8080", changeOrigin: false },
      "/sitemap.xml": { target: "http://127.0.0.1:8080", changeOrigin: false },
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: false },
      "/uploads": { target: "http://127.0.0.1:8080", changeOrigin: false },
    },
  },
});
