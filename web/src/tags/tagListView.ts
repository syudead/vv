import { t, type UiText } from "../i18n";
import { isFilteredView } from "../recordList/listView";
import type { TagPageRows } from "./tagPageRows";

/** Use server totals, excluding pagination boundaries and pinned renaming rows. */
export function tagCountText(page: TagPageRows | undefined): UiText {
  if (page === undefined) return t.tags.loading;
  return page.query.query !== "" || isFilteredView(page.query)
    ? t.tags.filteredCount(page.total, page.totalAll)
    : t.tags.count(page.totalAll);
}
