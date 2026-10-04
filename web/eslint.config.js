import { fileURLToPath } from "node:url";
import babelParser from "@babel/eslint-parser";
import js from "@eslint/js";
import betterTailwindcss from "eslint-plugin-better-tailwindcss";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import designExceptions from "./design-exceptions.js";

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

// デザインシステムの外の書き方を落とす（specs/038-design-system/contracts/registry.md
// の Check rules、research.md R-8）。例外はコメントでなく web/design-exceptions.js に置き、
// 一覧の検査は src/theme/designExceptions.test.ts が行う。
const sourceFiles = ["src/**/*.{ts,tsx}"];
const i18nFiles = ["src/i18n/**"];
const testFiles = ["src/**/*.test.{ts,tsx}", "src/testing/**"];
const designSystemFiles = ["src/ui/**"];

const controlRule = "no-restricted-syntax";
const restrictedClassesRule = "better-tailwindcss/no-restricted-classes";
const unknownClassesRule = "better-tailwindcss/no-unknown-classes";

const controlRestrictedSyntax = [
  {
    selector: "JSXOpeningElement[name.name=/^(button|input|select|textarea)$/]",
    message: "Use the design-system component (web/registry/rules/components.md).",
  },
];

// 変種（任意の変種 `data-[state=open]:` を含む）を先読みと後方参照で最後の `:` まで
// 取り切り、残ったユーティリティに `[` か `(` があれば、任意の値・任意のプロパティ・
// `(--var)` 省略形とみなす。
const variants = String.raw`(?=((?:(?:[^:\[\]]|\[[^\]]*\])*:)*))\1`;
const arbitraryUtility = String.raw`[^\[(]*[\[(]`;
const arbitraryMessage =
  "Arbitrary value outside the design-system scale (web/registry/rules/foundations.md).";

// 段階の外の数値の段（`p-7`、`gap-2.5`、`text-2xl`、`rounded-xl`、`font-bold` など）を落とす
// （contracts/registry.md の Check rules、ui-design.md の Foundations）。段階は
// src/ui/tokens.css と web/registry/rules/foundations.md にあり、この規則は最後の単位が
// テーマの名前空間を空にして no-unknown-classes に任せるまでの間だけ使う（research.md R-8）。
const spacingSteps = [
  "0",
  "0\\.5",
  "1",
  "1\\.5",
  "2",
  "3",
  "4",
  "5",
  "6",
  "8",
  "9",
  "10",
  "12",
  "16",
];
const spacingUtility = String.raw`-?(?:p[xytrblse]?|m[xytrblse]?|gap(?:-[xy])?|space-[xy]|w|h|size|min-[wh]|max-[wh]|inset(?:-[xy])?|top|right|bottom|left|start|end|translate-[xy]|basis|scroll-[mp][xytrblse]?|indent)`;
const offScaleUtility = [
  String.raw`${spacingUtility}-(?!(?:${spacingSteps.join("|")})$)\d+(?:\.\d+)?`,
  String.raw`text-(?:[2-9]xl)`,
  String.raw`rounded(?:-[trblse]{1,2})?-(?:xl|[2-9]xl)`,
  String.raw`font-(?:thin|extralight|light|bold|extrabold|black)`,
].join("|");
const offScaleMessage =
  "Step outside the design-system scale (web/registry/rules/foundations.md).";

function restrictedClasses(allowed = []) {
  const allow = allowed.length > 0 ? `(?!(?:${allowed.join("|")})$)` : "";
  return [
    "error",
    {
      restrict: [
        { pattern: `^${allow}${variants}${arbitraryUtility}`, message: arbitraryMessage },
        {
          pattern: `^${allow}${variants}(?:${offScaleUtility})$`,
          message: offScaleMessage,
        },
      ],
    },
  ];
}

function unknownClasses(allowed = []) {
  return ["error", { ignore: allowed.map((pattern) => `^(?:${pattern})$`) }];
}

// 例外の項目を、その 1 ファイルだけに効く規則の上書きにする。no-restricted-syntax は
// 訳し漏れの検査と共有しているため、生の部品の選択子だけを外す。
function exceptionRules(entry) {
  const rules = {};
  for (const rule of entry.rules) {
    if (rule === controlRule) {
      rules[rule] = entry.file.startsWith("i18n/")
        ? "off"
        : ["error", ...i18nRestrictedSyntax];
    } else if (entry.classes === undefined) {
      rules[rule] = "off";
    } else if (rule === restrictedClassesRule) {
      rules[rule] = restrictedClasses(entry.classes);
    } else if (rule === unknownClassesRule) {
      rules[rule] = unknownClasses(entry.classes);
    }
  }
  return rules;
}

function designSystemConfig() {
  return [
    {
      files: sourceFiles,
      ignores: testFiles,
      plugins: { "better-tailwindcss": betterTailwindcss },
      settings: {
        "better-tailwindcss": {
          entryPoint: fileURLToPath(new URL("./src/index.css", import.meta.url)),
        },
      },
      rules: {
        [restrictedClassesRule]: restrictedClasses(),
        [unknownClassesRule]: unknownClasses(),
      },
    },
    {
      files: sourceFiles,
      ignores: [...i18nFiles, ...testFiles, ...designSystemFiles],
      rules: {
        [controlRule]: ["error", ...i18nRestrictedSyntax, ...controlRestrictedSyntax],
      },
    },
    {
      files: i18nFiles,
      ignores: testFiles,
      rules: {
        [controlRule]: ["error", ...controlRestrictedSyntax],
      },
    },
    ...designExceptions.map((entry) => ({
      files: [`src/${entry.file}`],
      ignores: testFiles,
      rules: exceptionRules(entry),
    })),
  ];
}

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
      "design-exceptions.d.ts",
    ],
    rules: {
      "no-undef": "off",
      "no-unused-vars": "off",
    },
  },
  {
    files: sourceFiles,
    ignores: [...i18nFiles, ...testFiles],
    rules: {
      "no-restricted-syntax": ["error", ...i18nRestrictedSyntax],
    },
  },
  ...designSystemConfig(),
];
