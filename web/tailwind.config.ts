import type { Config } from "tailwindcss";

// src/index.css loads this config for scan targets; Tailwind v4 theme values live in CSS.
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
} satisfies Config;
