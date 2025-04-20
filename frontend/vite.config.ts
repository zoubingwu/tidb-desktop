import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tsconfigPaths from "vite-tsconfig-paths";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), tsconfigPaths()],
  // https://github.com/wailsapp/wails/issues/3064#issuecomment-2053632869
  server: {
    hmr: {
      host: "localhost",
      protocol: "ws",
    },
  },
});
