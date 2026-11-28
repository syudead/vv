// A directive starts the comment, or a line of a block comment after its leading asterisk.
const directive = /(?:^|\n)[\s*]*@ts-(ignore|expect-error|nocheck)\b/u;

/** @type {import("eslint").Rule.RuleModule} */
export default {
  meta: {
    type: "problem",
    docs: { description: "Reject comments that silence the TypeScript checker" },
    schema: [],
    messages: {
      suppressed:
        "@ts-{{name}} silences a type error; fix the code instead, since findings are never silenced (docs/how-to/development.md).",
    },
  },
  create(context) {
    return {
      Program() {
        for (const comment of context.sourceCode.getAllComments()) {
          const match = directive.exec(comment.value);
          if (match !== null) {
            context.report({
              loc: comment.loc,
              messageId: "suppressed",
              data: { name: match[1] },
            });
          }
        }
      },
    };
  },
};
