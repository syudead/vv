import type { WatchHistoryEntry } from "../api/history";
import { formatDate, t, type UiText } from "../i18n";

// 視聴履歴の日ごとのまとまり（specs/043-watch-history/ui-design.md「Day groups」）。
// 日の境目は見る人のブラウザの時間帯の 0 時。

/** HistoryDay は 1 日分のまとまりである。`entries` は API の順（新しい順）のまま。 */
export interface HistoryDay {
  /** その日の 0 時（ローカル）。 */
  day: Date;
  /** まとまりの鍵（`2026-9-27` の形）。 */
  key: string;
  entries: WatchHistoryEntry[];
}

function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function dayKey(date: Date): string {
  return `${String(date.getFullYear())}-${String(date.getMonth() + 1)}-${String(date.getDate())}`;
}

/**
 * groupByDay は件を `playedAt` のローカルの日でまとめる。まとまりは最初に現れた順、つまり
 * 新しい日が先になり、続きのページの件が同じ日なら開いているまとまりに加わるので、同じ日が
 * 2 度出ない。読めない時刻の件はまとめない。
 */
export function groupByDay(entries: readonly WatchHistoryEntry[]): HistoryDay[] {
  const days = new Map<string, HistoryDay>();
  for (const entry of entries) {
    const played = new Date(entry.playedAt);
    if (Number.isNaN(played.getTime())) continue;
    const key = dayKey(played);
    const group = days.get(key);
    if (group === undefined) {
      days.set(key, { day: startOfDay(played), key, entries: [entry] });
    } else {
      group.entries.push(entry);
    }
  }
  return Array.from(days.values());
}

/** dayLabel はまとまりの見出しである。今日は「Today」、前の日は「Yesterday」、ほかは日付。 */
export function dayLabel(day: Date, now: Date): UiText {
  const today = startOfDay(now);
  if (dayKey(day) === dayKey(today)) return t.history.day.today;
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1);
  if (dayKey(day) === dayKey(yesterday)) return t.history.day.yesterday;
  return formatDate(day);
}

/** msUntilNextMidnight は `now` から次のローカルの 0 時までのミリ秒である。 */
export function msUntilNextMidnight(now: Date): number {
  const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  return Math.max(0, next.getTime() - now.getTime());
}
