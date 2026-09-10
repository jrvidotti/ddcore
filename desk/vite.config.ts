import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://localhost:8090", changeOrigin: false },
      "/assets": "http://localhost:8090",
      "/files": "http://localhost:8090",
      "/private": "http://localhost:8090",
    },
  },
});
