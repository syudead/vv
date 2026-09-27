import babelParser from "@babel/eslint-parser";
import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";

export default [
  {
    ignores: ["dist/**", ".vite-build-check/**", "test-results/**", "src/api/gen/**"],
  },
  {
    files: ["**/*.{js,mjs,cjs,ts,tsx}"],
    languageOptions: {
      parser: babelParser,
      parserOptions: {
        requireConfigFile: false,
        babelOptions: {
          parserOpts: {
            plugins: ["typescript", "jsx"],
          },
        },
      },
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
    plugins: {
      "react-hooks": reactHooks,
    },
    rules: {
      ...js.configs.recommended.rules,
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "error",
    },
  },
  {
    files: [
      "src/**/*.{ts,tsx}",
      "e2e/**/*.ts",
      "vite.config.ts",
      "tailwind.config.ts",
      "vitest.setup.ts",
      "playwright.config.ts",
    ],
    rules: {
      "no-undef": "off",
      "no-unused-vars": "off",
    },
  },
];
