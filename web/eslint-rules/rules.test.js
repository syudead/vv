// @vitest-environment node
import { RuleTester } from "eslint";

import englishComments from "./english-comments.js";
import noTsSuppression from "./no-ts-suppression.js";

const tester = new RuleTester({
  languageOptions: { ecmaVersion: 2022, sourceType: "module" },
});

tester.run("english-comments", englishComments, {
  valid: [
    "// Keep the focus on the heading.",
    // Kanji and katakana alone are names, such as a sample title.
    "// The sample tag is 旅行 and the button reads ボタン.",
    "// The label reads `取り込み中です。` until the scan ends.",
    "const label = 'ひらがなの文字列、文字列です。';",
  ],
  invalid: [
    { code: "// 一覧を読み直す", errors: [{ messageId: "japanese" }] },
    { code: "/* Reads the list、then focuses. */", errors: [{ messageId: "japanese" }] },
    {
      code: "/**\n * 終わった。\n */\nconst a = 1;",
      errors: [{ messageId: "japanese" }],
    },
    {
      code: "// `quoted` text but 読み直す outside",
      errors: [{ messageId: "japanese" }],
    },
  ],
});

tester.run("no-ts-suppression", noTsSuppression, {
  valid: ["// Mentions ts-ignore without the directive.", "/* see @ts-check docs */"],
  invalid: [
    {
      code: "// @ts-ignore\nconst a = 1;",
      errors: [{ messageId: "suppressed", data: { name: "ignore" } }],
    },
    {
      code: "// @ts-expect-error: wrong type\nconst a = 1;",
      errors: [{ messageId: "suppressed", data: { name: "expect-error" } }],
    },
    {
      code: "/* @ts-nocheck */\nconst a = 1;",
      errors: [{ messageId: "suppressed", data: { name: "nocheck" } }],
    },
    {
      code: "/**\n * @ts-ignore\n */\nconst a = 1;",
      errors: [{ messageId: "suppressed", data: { name: "ignore" } }],
    },
  ],
});
