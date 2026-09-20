import type { VideoSort } from "../api/client";

/** Zoom はカードの大きさ。0 が最小、3 が最大（Stash のズームスライダーと同じ 4 段）。 */
export type Zoom = 0 | 1 | 2 | 3;

/** ViewMode は一覧の表示形式。 */
export type ViewMode = "grid" | "list";

export interface ViewPreferences {
  zoom: Zoom;
  view: ViewMode;
  sort: VideoSort;
}

const storageKey = "vv.view.v2";

export const defaults: ViewPreferences = { zoom: 1, view: "grid", sort: "addedDesc" };

const sorts: Record<VideoSort, true> = { addedDesc: true, titleAsc: true };
const views: Record<ViewMode, true> = { grid: true, list: true };

function isKeyOf<T extends string>(table: Record<T, true>, value: unknown): value is T {
  return typeof value === "string" && Object.hasOwn(table, value);
}

function isZoom(value: unknown): value is Zoom {
  return value === 0 || value === 1 || value === 2 || value === 3;
}

/**
 * readViewPreferences はどの場合でも投げず、完全な値を返す。
 * 壊れている項目だけを既定値へ落とす（zoom が壊れても並び順は残す）。
 */
export function readViewPreferences(storage?: Storage): ViewPreferences {
  let raw: string | null;
  try {
    raw = (storage ?? window.localStorage).getItem(storageKey);
  } catch {
    return { ...defaults };
  }
  if (raw === null) return { ...defaults };

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { ...defaults };
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return { ...defaults };
  }

  const value = parsed as Record<string, unknown>;
  return {
    zoom: isZoom(value.zoom) ? value.zoom : defaults.zoom,
    view: isKeyOf(views, value.view) ? value.view : defaults.view,
    sort: isKeyOf(sorts, value.sort) ? value.sort : defaults.sort,
  };
}

export function writeViewPreferences(value: ViewPreferences, storage?: Storage): void {
  try {
    const stored: ViewPreferences = {
      zoom: value.zoom,
      view: value.view,
      sort: value.sort,
    };
    (storage ?? window.localStorage).setItem(storageKey, JSON.stringify(stored));
  } catch {
    // 保存できなくても今の画面では設定を使える
  }
}
