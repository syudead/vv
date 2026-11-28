// vv's own ESLint rules, for documented rules no core or plugin rule covers.
import englishComments from "./english-comments.js";
import noTsSuppression from "./no-ts-suppression.js";

export default {
  meta: { name: "vv" },
  rules: {
    "english-comments": englishComments,
    "no-ts-suppression": noTsSuppression,
  },
};
