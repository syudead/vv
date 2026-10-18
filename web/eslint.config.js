import { fileURLToPath } from "node:url";
import babelParser from "@babel/eslint-parser";
import js from "@eslint/js";
import vitest from "@vitest/eslint-plugin";
import betterTailwindcss from "eslint-plugin-better-tailwindcss";
import reactHooks from "eslint-plugin-react-hooks";
import testingLibrary from "eslint-plugin-testing-library";
import globals from "globals";
import designExceptions from "./design-exceptions.js";

// UI text and formatting belong exclusively to src/i18n.
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

// Centralize design exceptions in design-exceptions.js; src/theme/designExceptions.test.ts validates them.
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
  // Reserve style for runtime measurements and positions; fixed values must use tokens and classes.
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

// Consume variants through the final colon, including one nested bracket level, before detecting arbitrary values, properties or (--var) shorthand in the utility.
const variants = String.raw`(?=((?:(?:[^:\[\]]|\[(?:[^\[\]]|\[[^\]]*\])*\])*:)*))\1`;
const arbitraryUtility = String.raw`[^\[(]*[\[(]`;
const arbitraryMessage =
  "Arbitrary value outside the design-system scale (web/registry/rules/foundations.md).";

// Reset theme namespaces make off-scale numeric classes unknown. Preserve the shared focus ring; upstream shadcn components are exempt (web/registry/rules/components.md).
const focusRing = String.raw`(?:^|:)!?outline-(?:none|hidden)!?$|(?:^|:)(?:focus|focus-visible|focus-within):(?:[^:]*:)*!?(?:ring|outline)(?:-[^:]*)?!?$`;
const focusMessage =
  "Keep the shared focus ring (web/src/index.css :focus-visible); never remove it or add your own (web/registry/rules/components.md, Focus).";

function restrictedClasses(allowed = [], { focus = true } = {}) {
  // Ignore both v3 prefix and v4 suffix ! markers when matching the allowlist.
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

// Override raw-component selectors only; no-restricted-syntax also enforces translation rules.
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
    // Reject inline suppression comments; --max-warnings 0 turns their warnings into lint failures.
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
  // Use recommended test rules with the exceptions below; see docs/design-docs/web-testing.md.
  {
    files: testFiles,
    plugins: vitest.configs.recommended.plugins,
    rules: {
      ...vitest.configs.recommended.rules,
      // Treat expect-prefixed assertion helpers as assertions.
      "vitest/expect-expect": ["error", { assertFunctionNames: ["expect", "expect*"] }],
      // Vitest accepts a failure message as the second expect argument, used for table-row names.
      "vitest/valid-expect": ["error", { maxArgs: 2 }],
    },
  },
  {
    files: testFiles,
    plugins: testingLibrary.configs["flat/react"].plugins,
    rules: {
      ...testingLibrary.configs["flat/react"].rules,
      // Structural assertions inspect data markers, focus targets, video elements and siblings without accessible roles.
      "testing-library/no-node-access": "off",
      // Container queries inspect structural markers and icon classes without accessible roles.
      "testing-library/no-container": "off",
      // Allow explicit cleanup in vitest.setup.ts and before rerendering within a test.
      "testing-library/no-manual-cleanup": "off",
      // Scenario-specific result names improve multi-render tests; render-named helpers do not always return render results.
      "testing-library/render-result-naming-convention": "off",
    },
  },
  {
    // setRenderedAudience is production code misidentified as a render helper; limit this exception to its callers.
    files: [
      "src/api/serverEvents.test.ts",
      "src/player/VideoPage.test.tsx",
      "src/player/VideoPagePlaylist.test.tsx",
    ],
    rules: {
      "testing-library/no-render-in-lifecycle": "off",
    },
  },
  {
    // This keyboard test has no readiness signal for its replacement player effect; retry the idempotent 0 key until it takes effect.
    files: ["src/player/VideoPage.test.tsx"],
    rules: {
      "testing-library/no-wait-for-side-effects": "off",
    },
  },
];
