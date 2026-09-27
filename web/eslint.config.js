import babelParser from "@babel/eslint-parser";
import js from "@eslint/js";
import jsxA11y from "eslint-plugin-jsx-a11y";
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
  {
    selector: 'CallExpression[callee.name="untranslated"]',
    message: "untranslated() is only for directories on the i18n exclusion list.",
  },
];

// まだ英語のカタログへ移していないディレクトリである。各領域の単位が自分の
// ディレクトリをここから外し、最後の単位で一覧を無くす（R-3）。
const untranslatedDirectories = [
  "src/auth/**",
  "src/shell/**",
  "src/settings/**",
  "src/library/**",
  "src/videoList/**",
  "src/folders/**",
  "src/tags/**",
  "src/player/**",
];

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
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/i18n/**", "src/**/*.test.{ts,tsx}", ...untranslatedDirectories],
    rules: {
      "no-restricted-syntax": ["error", ...i18nRestrictedSyntax],
    },
  },
];
