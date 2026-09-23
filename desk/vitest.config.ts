import path from "node:path";
import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";

const alias = { $lib: path.resolve(__dirname, "./src/lib") };

export default defineConfig({
  plugins: [svelte()],
  resolve: { alias },
  test: {
    projects: [
      {
        extends: true,
        test: { name: "node", include: ["src/**/*.test.ts"], exclude: ["src/**/*.dom.test.ts"], environment: "node" },
      },
      {
        // Components mounted in a DOM, so their effects run: Svelte's client build.
        extends: true,
        resolve: { alias, conditions: ["browser"] },
        test: { name: "dom", include: ["src/**/*.dom.test.ts", "src/**/*.dom.test.svelte.ts"], environment: "jsdom" },
      },
    ],
  },
});
