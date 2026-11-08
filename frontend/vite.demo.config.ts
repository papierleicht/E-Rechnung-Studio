import { fileURLToPath } from "node:url";
import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Browser live demo for papierleicht.de: the Wails runtime is replaced by an
// in-memory backend (src/mock), so the build runs without the Go app. The
// build stays in frontend/dist-demo unless DEMO_OUT_DIR names another folder,
// as the website's own build does.
const outDir = process.env.DEMO_OUT_DIR || "dist-demo";

// The website embeds /demo-app/index.html.
const indexPage: Plugin = {
  name: "demo-index-page",
  enforce: "post",
  generateBundle(_, bundle) {
    const page = bundle["demo.html"];
    if (page) page.fileName = "index.html";
  },
};

export default defineConfig({
  base: "/demo-app/",
  resolve: {
    alias: [
      {
        find: /^@wailsio\/runtime$/,
        replacement: fileURLToPath(new URL("./src/mock/runtime.ts", import.meta.url)),
      },
    ],
  },
  plugins: [svelte(), indexPage],
  build: {
    outDir,
    emptyOutDir: true,
    rollupOptions: { input: fileURLToPath(new URL("./demo.html", import.meta.url)) },
  },
  server: { host: "127.0.0.1", port: 9246, strictPort: true, open: "/demo-app/demo.html" },
});
