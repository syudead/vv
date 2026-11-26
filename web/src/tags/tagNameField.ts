import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import type { TagFieldError } from "../ui/nameField";

export interface TagFieldContext {
  submitted?: string;
  ownTagName?: string;
}

function takenText(failure: RequestFailed, context: TagFieldContext): UiText {
  const owner = failure.tagName;
  if (owner === undefined) return errorText(failure);
  const own = context.ownTagName !== undefined && owner === context.ownTagName;
  if (failure.reason === "name_is_tag") {
    return own ? t.tags.nameIsOwnName(owner) : t.tags.nameIsTag(owner);
  }
  if (failure.reason === "name_is_synonym" && context.submitted !== undefined) {
    return own
      ? t.tags.nameIsOwnSynonym(context.submitted)
      : t.tags.nameIsSynonym(context.submitted, owner);
  }
  return errorText(failure);
}

/** Classify name conflicts separately and never display raw server messages. */
export function tagFieldError(
  failure: unknown,
  context: TagFieldContext = {},
): TagFieldError {
  if (failure instanceof RequestFailed && failure.code === "tag_name_taken") {
    return { kind: "taken", message: takenText(failure, context) };
  }
  return { kind: "other", message: errorText(failure) };
}

const kindNameReasons: ReadonlySet<string> = new Set([
  "tag_kind_name_taken",
  "tag_name_empty",
  "tag_name_control_characters",
  "tag_name_too_long",
]);

export function kindFieldError(failure: unknown): TagFieldError {
  const byName =
    failure instanceof RequestFailed &&
    failure.reason !== undefined &&
    kindNameReasons.has(failure.reason);
  return { kind: byName ? "taken" : "other", message: errorText(failure) };
}
