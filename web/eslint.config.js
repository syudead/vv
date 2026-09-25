import babelParser from "@babel/eslint-parser";
import js from "@eslint/js";
import jsxA11y from "eslint-plugin-jsx-a11y";
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
      "jsx-a11y": jsxA11y,
      "react-hooks": reactHooks,
    },
    rules: {
      ...js.configs.recommended.rules,
      ...jsxA11y.flatConfigs.recommended.rules,
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
  {
    // These containers only observe clicks delegated to their descendant controls.
    files: [
      "src/folders/FolderView.tsx",
      "src/folders/RootSearchResults.tsx",
      "src/library/CardTagRow.tsx",
      "src/library/LibraryPage.tsx",
    ],
    rules: {
      "jsx-a11y/click-events-have-key-events": "off",
      "jsx-a11y/no-static-element-interactions": "off",
    },
  },
  {
    // The input owns keyboard interaction for the listbox options.
    files: ["src/ui/Combobox.tsx"],
    rules: {
      "jsx-a11y/click-events-have-key-events": "off",
    },
  },
  {
    // The focusable list region handles keyboard events for its child buttons.
    files: ["src/settings/FolderPicker.tsx"],
    rules: {
      "jsx-a11y/no-static-element-interactions": "off",
    },
  },
];
