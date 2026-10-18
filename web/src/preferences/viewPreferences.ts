import type { VideoSort } from "../api/client";
import { isListSort } from "../videoList/listCriteria";

export type Zoom = 0 | 1 | 2 | 3;

export type ViewMode = "grid" | "list";

export interface ViewPreferences {
  zoom: Zoom;
  view: ViewMode;
  sort: VideoSort;
}

const storageKey = "vv.view.v2";

export const defaults: ViewPreferences = { zoom: 1, view: "grid", sort: "addedDesc" };

// Restore only supported menu sorts; never persist random seeds so each opening creates a new one.
const views: Record<ViewMode, true> = { grid: true, list: true };

function isKeyOf<T extends string>(table: Record<T, true>, value: unknown): value is T {
  return typeof value === "string" && Object.hasOwn(table, value);
}

function isZoom(value: unknown): value is Zoom {
  return value === 0 || value === 1 || value === 2 || value === 3;
}

/** Never throw; default only invalid fields so valid preferences survive partial corruption. */
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
    sort: isListSort(value.sort) ? value.sort : defaults.sort,
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
    // Keep the current preferences even when persistence fails.
  }
}
