import babelParser from "@babel/eslint-parser";
import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";

// 画面の文言の訳し漏れを報告する（specs/023-english-i18n/research.md R-3、
// docs/design-docs/i18n.md）。文言と書式は web/src/i18n/ だけが持つ。
const japanese =
  "[\\u3040-\\u30ff\\u3400-\\u4dbf\\u4e00-\\u9fff\\uf900-\\ufaff\\uff66-\\uff9f]";
const letter =
  "[A-Za-z\\u00c0-\\u024f\\u3040-\\u30ff\\u3400-\\u4dbf\\u4e00-\\u9fff\\uf900-\\ufaff\\uff66-\\uff9f]";
const textAttribute = "JSXAttribute[name.name=/^(aria-label|title|placeholder|alt)$/]";
const catalogMessage =
  "Put user-facing text in the i18n catalog (web/src/i18n) and use t.";
const i18nRestrictedSyntax = [
  {
    selector: `Literal[value=/${japanese}/]`,
    message: `Japanese text outside web/src/i18n. ${catalogMessage}`,
  },
  {
    selector: `TemplateElement[value.raw=/${japanese}/]`,
    message: `Japanese text outside web/src/i18n. ${catalogMessage}`,
  },
  {
    selector: `JSXText[value=/${letter}/]`,
    message: `Fixed JSX text. ${catalogMessage}`,
  },
  {
    selector: `${textAttribute} > Literal[value=/\\S/]`,
    message: `Fixed accessible name or hint. ${catalogMessage}`,
  },
  {
    selector: `${textAttribute} > JSXExpressionContainer > Literal[value=/\\S/]`,
    message: `Fixed accessible name or hint. ${catalogMessage}`,
  },
  {
    selector: `${textAttribute} > JSXExpressionContainer > TemplateLiteral > TemplateElement[value.raw=/${letter}/]`,
    message: `Fixed accessible name or hint. ${catalogMessage}`,
  },
  {
    selector: 'Literal[value="ja-JP"]',
    message: "Format dates and numbers with the functions in web/src/i18n.",
  },
  {
    selector:
      "CallExpression[arguments.length=0][callee.property.name=/^toLocale(Date|Time)?String$/]",
    message: "Format dates and numbers with the functions in web/src/i18n.",
  },
];

export default [
  {
    ignores: ["dist/**", ".vite-build-check/**", "test-results/**", "src/api/gen/**"],
  },
  {
    // 規則はコード全体に例外なく効かせる。指摘はコードの書き換えで解消し、
    // eslint-disable* などのコメントによる無効化は認めない（syudead/vv#443）。
    // 書かれたコメントは効かず警告になり、lint の --max-warnings 0 で失敗する。
    linterOptions: {
      noInlineConfig: true,
    },
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
      "bench/**/*.ts",
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
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/i18n/**", "src/**/*.test.{ts,tsx}"],
    rules: {
      "no-restricted-syntax": ["error", ...i18nRestrictedSyntax],
    },
  },
];
