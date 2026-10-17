import type { HomeOrganizer, HomeOrganizeRun } from "../api/home";
import { formatDateTime, t, type UiText } from "../i18n";

// 設定の「Home」の節の規則（specs/057-home/ui-design.md「Words」の Settings の行）。画面の部品から
// 外し、ロジックのテストで確かめる（docs/design-docs/web-testing.md）。

/** lastRunLine は「Shelves」の説明の下に出す、最後の実行の行である（ui-design.md「Words」）。 */
export function lastRunLine(last: HomeOrganizeRun | undefined): UiText {
  const text = t.settings.home.lastRun;
  if (last === undefined) return text.never;
  const when = formatDateTime(new Date(last.finishedAt));
  switch (last.outcome) {
    case "failed":
      return text.failed(when);
    case "unchanged":
      return text.unchanged(when);
    case "done":
      return last.added === 0 && last.removed === 0
        ? text.unchanged(when)
        : text.changed(when, last.added, last.removed);
  }
}

/** organizerBusy は実行が待ちにあるか走っているかである。その間「Reorganize」を押せない。 */
export function organizerBusy(organizer: HomeOrganizer): boolean {
  return organizer.running || organizer.pending !== undefined;
}
