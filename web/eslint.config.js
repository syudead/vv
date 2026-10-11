import { fileURLToPath } from "node:url";
import babelParser from "@babel/eslint-parser";
import js from "@eslint/js";
import vitest from "@vitest/eslint-plugin";
import betterTailwindcss from "eslint-plugin-better-tailwindcss";
import reactHooks from "eslint-plugin-react-hooks";
import testingLibrary from "eslint-plugin-testing-library";
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
    selector:
      "JSXOpeningElement[name.name=/^(button|input|select|textarea|table|dialog)$/]",
    message: "Use the design-system component (web/registry/rules/components.md).",
  },
  {
    selector: 'JSXAttribute[name.name="role"][value.value="button"]',
    message: "Use the design-system Button (web/registry/rules/components.md).",
  },
  // style は実行時にしか決まらない値（位置・割合・測った高さ）だけに使う。固定の値は段の外の
  // 値になるので、トークンとクラスで書く。
  {
    selector: 'JSXAttribute[name.name="style"] Property[value.type="Literal"]',
    message:
      "A fixed value in style bypasses the design-system scale. Use the tokens and classes (web/registry/rules/foundations.md); style is only for values known at run time.",
  },
  {
    selector:
      'JSXAttribute[name.name="style"] Property[value.type="UnaryExpression"][value.argument.type="Literal"]',
    message:
      "A fixed value in style bypasses the design-system scale. Use the tokens and classes (web/registry/rules/foundations.md); style is only for values known at run time.",
  },
  {
    selector:
      'JSXAttribute[name.name="style"] Property[value.type="TemplateLiteral"][value.expressions.length=0]',
    message:
      "A fixed value in style bypasses the design-system scale. Use the tokens and classes (web/registry/rules/foundations.md); style is only for values known at run time.",
  },
  {
    selector: "ImportDeclaration[source.value=/^(radix-ui|@radix-ui\\/)/]",
    message:
      "Use the design-system components in web/src/ui/shadcn instead of Radix directly.",
  },
];

// 変種（任意の変種 `data-[state=open]:` を含む）を先読みと後方参照で最後の `:` まで
// 取り切り、残ったユーティリティに `[` か `(` があれば、任意の値・任意のプロパティ・
// `(--var)` 省略形とみなす。任意の変種の中の 1 段の入れ子（shadcn/ui の
// `[&_svg:not([class*='size-'])]:`）も変種として読む。
const variants = String.raw`(?=((?:(?:[^:\[\]]|\[(?:[^\[\]]|\[[^\]]*\])*\])*:)*))\1`;
const arbitraryUtility = String.raw`[^\[(]*[\[(]`;
const arbitraryMessage =
  "Arbitrary value outside the design-system scale (web/registry/rules/foundations.md).";

// 段階の外の数値の段（`p-7`、`text-2xl`、`rounded-xl`、`font-bold` など）は、src/ui/tokens.css
// がテーマの名前空間を空にしているので生成されず、no-unknown-classes が落とす（research.md R-8）。
// 共通のフォーカスの輪（index.css の :focus-visible）を消すか、独自の輪を足すクラス。
// web/registry/rules/components.md の Focus。shadcn の部品（ui/shadcn）は上流のまま持つので外す。
const focusRing = String.raw`(?:^|:)!?outline-(?:none|hidden)!?$|(?:^|:)(?:focus|focus-visible|focus-within):(?:[^:]*:)*!?(?:ring|outline)(?:-[^:]*)?!?$`;
const focusMessage =
  "Keep the shared focus ring (web/src/index.css :focus-visible); never remove it or add your own (web/registry/rules/components.md, Focus).";

function restrictedClasses(allowed = [], { focus = true } = {}) {
  // 末尾（v4）と先頭（v3）の重要度の印 `!` は許す一覧の判定から外す。
  const allow = allowed.length > 0 ? `(?!!?(?:${allowed.join("|")})!?$)` : "";
  const restrict = [
    { pattern: `^${allow}${variants}${arbitraryUtility}`, message: arbitraryMessage },
  ];
  if (focus)
    restrict.push({ pattern: `^${allow}.*?(?:${focusRing})`, message: focusMessage });
  return ["error", { restrict }];
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
      rules[rule] = restrictedClasses(entry.classes, {
        focus: !entry.file.startsWith("ui/shadcn/"),
      });
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
      files: ["src/ui/shadcn/**"],
      ignores: testFiles,
      rules: {
        [restrictedClassesRule]: restrictedClasses([], { focus: false }),
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
  // テストコードの質を見る（docs/design-docs/web-testing.md）。両方の推奨設定から始め、
  // 変える規則と外す規則は理由をここに書く。
  {
    files: testFiles,
    plugins: vitest.configs.recommended.plugins,
    rules: {
      ...vitest.configs.recommended.rules,
      // expectCatalogTextOnly のような expect で始まる補助関数も表明として数える。
      "vitest/expect-expect": ["error", { assertFunctionNames: ["expect", "expect*"] }],
      // Vitest の expect は 2 つ目の引数に失敗時の文言を取る。表の検査はその行の名前を渡す。
      "vitest/valid-expect": ["error", { maxArgs: 2 }],
    },
  },
  {
    files: testFiles,
    plugins: testingLibrary.configs["flat/react"].plugins,
    rules: {
      ...testingLibrary.configs["flat/react"].rules,
      // 約 700 件（48 ファイル）。役割や名前を持たない構造（data-* の目印、フォーカスの
      // 移動先、<video>、隣の要素）を確かめるテストが DOM を辿っており、役割の問い合わせ
      // へ書き換えられない箇所が多い。書き換えは振る舞いを変えない大量の差分になる。
      "testing-library/no-node-access": "off",
      // no-node-access と同じ理由。container から引くのは data-* の目印やアイコンの
      // クラスで、役割の問い合わせに置き換えられない。
      "testing-library/no-container": "off",
      // Vitest の globals を切っているので、Testing Library の自動の後片付けは動かない。
      // 後片付けは vitest.setup.ts が明示的に呼び、テストの途中で描き直す前にも呼ぶ。
      "testing-library/no-manual-cleanup": "off",
      // 1 つのテストで何度も描くときは first・direct・loading のように場面の名前を付ける
      // ほうが読める。また render を名前に含む補助関数（onClose を返す renderHeader など）
      // まで描画の結果とみなして誤って指摘する。
      "testing-library/render-result-naming-convention": "off",
    },
  },
  {
    // 本番の関数 setRenderedAudience（src/api/client.ts）を、名前に render を含むので
    // 描画とみなして誤って指摘する。描画の補助関数の名前を列挙して判定を狭めると、補助
    // 関数を足すたびに設定を直すことになるので、呼んでいる 2 ファイルでだけ切る。
    files: ["src/api/serverEvents.test.ts", "src/player/VideoPage.test.tsx"],
    rules: {
      "testing-library/no-render-in-lifecycle": "off",
    },
  },
  {
    // 「画面のどこでも Space・0 がプレイヤーに効く」は、画面全体のキー操作が差し替えた
    // プレイヤーの操作を受け取る effect を待つ目印を持たないので、何度押しても同じ結果に
    // なる 0 を、効くまで waitFor の中で押し直す。その 1 件のためにこのファイルでだけ切る。
    files: ["src/player/VideoPage.test.tsx"],
    rules: {
      "testing-library/no-wait-for-side-effects": "off",
    },
  },
];
