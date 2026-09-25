import babelParser from "@babel/eslint-parser";
import js from "@eslint/js";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";

export default [
  {
    ignores: ["dist/**", ".vite-build-check/**", "test-results/**"],
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
      // Delegated containers and composite widgets provide keyboard handling elsewhere.
      "jsx-a11y/click-events-have-key-events": "off",
      "jsx-a11y/no-static-element-interactions": "off",
      "no-undef": "off",
      "no-unused-vars": "off",
    },
  },
];
