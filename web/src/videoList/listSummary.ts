import { t, type UiText } from "../i18n";

export function resultCountText(total: number): UiText {
  return t.list.resultCount(total);
}
