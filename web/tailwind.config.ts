import type { Config } from "tailwindcss";

// Tailwind CSS 4 は設定を CSS 側（src/index.css の @theme）に置けるが、
// 走査対象だけはここで明示しておく。src/index.css の @config から読み込まれる。
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
} satisfies Config;
