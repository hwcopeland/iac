import { defineConfig } from "vite";

// Served from the root of a dedicated host (ptable.hwcopeland.net) behind nginx.
export default defineConfig({
  base: "/",
  build: {
    target: "es2021",
    sourcemap: false,
  },
  test: {
    globals: true,
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
  },
});
