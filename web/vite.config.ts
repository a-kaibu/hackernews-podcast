import { defineConfig } from "vite-plus";

export default defineConfig({
  base: "/hackernews-podcast/",
  lint: { options: { typeAware: true, typeCheck: true } },
});
