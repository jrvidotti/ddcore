import adapter from "@sveltejs/adapter-static";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

/** @type {import('@sveltejs/kit').Config} */
export default {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({ pages: "build", assets: "build", fallback: "index.html", precompress: false, strict: false }),
    alias: {
      $lib: "src/lib",
      // types only: the runtime is window.__ddcoreDesk, so nothing is imported from these at run time
      "@ddcore/sdk": "../packages/sdk/src/index.ts",
      "@ddcore/desk-sdk": "../packages/desk-sdk/src/index.ts",
    },
  },
};
