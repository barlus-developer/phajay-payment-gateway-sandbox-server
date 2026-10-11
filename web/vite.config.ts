import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": new URL("./src", import.meta.url).pathname } },
  server: {
    proxy: { "/api": "http://localhost:8080", "/v1": "http://localhost:8080" },
  },
  build: { sourcemap: false },
});
