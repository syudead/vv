// Japanese prose: a Japanese full stop or comma, two hiragana in a row, or a hiragana beside a kanji.
// Kanji or katakana alone are usually sample names; text in backticks is quoted, not prose.
const hiragana = String.raw`\u3041-\u309f`;
const kanji = String.raw`\u4e00-\u9fff`;
const japaneseProse = new RegExp(
  String.raw`[\u3001\u3002]|[${hiragana}]{2}|[${kanji}][${hiragana}]|[${hiragana}][${kanji}]`,
  "u",
);
const quoted = /`[^`\n]*`/gu;

/** @type {import("eslint").Rule.RuleModule} */
export default {
  meta: {
    type: "suggestion",
    docs: { description: "Write comments in English" },
    schema: [],
    messages: {
      japanese:
        "Write comments in concise English; quote Japanese text in backticks (AGENTS.md, Code comments).",
    },
  },
  create(context) {
    return {
      Program() {
        for (const comment of context.sourceCode.getAllComments()) {
          if (japaneseProse.test(comment.value.replace(quoted, ""))) {
            context.report({ loc: comment.loc, messageId: "japanese" });
          }
        }
      },
    };
  },
};
