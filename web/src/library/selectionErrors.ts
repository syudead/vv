import { RequestFailed } from "../api/client";
import { maxVideoTagsSelection } from "../api/tags";
import { t, type UiText } from "../i18n";

export function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

/** Atomic limits require both closing oversized popovers and checking the latest count on submission. Share the reason across tag and visibility actions. */
export function overLimitMessage(): UiText {
  return t.errors.reason.too_many_videos({ limit: maxVideoTagsSelection });
}
