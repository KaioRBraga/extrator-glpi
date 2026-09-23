import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Em desenvolvimento o front roda no Vite e as chamadas /api vao para o
// binario Go em HTTPS com certificado autoassinado -- dai o secure: false.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.VITE_API_ALVO ?? "https://localhost:8443",
        changeOrigin: true,
        secure: false,
      },
    },
  },
});
