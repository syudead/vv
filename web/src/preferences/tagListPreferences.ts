import {
  defaultTagListSort,
  isTagListSort,
  type TagListSort,
} from "../tags/tagListOrder";

/** Persist only sort order, never filters or queries; credits use separate keys with the same shape. */
export interface TagListPreferences {
  sort: TagListSort;
}

const storageKey = "vv.tags.v1";

export const tagListDefaults: TagListPreferences = { sort: defaultTagListSort };

/** Never throw; return complete defaults for unreadable or invalid storage. */
export function readTagListPreferences(storage?: Storage): TagListPreferences {
  return readListSortPreferences(storageKey, storage);
}

export function writeTagListPreferences(
  value: TagListPreferences,
  storage?: Storage,
): void {
  writeListSortPreferences(storageKey, value, storage);
}

export function readListSortPreferences(
  key: string,
  storage?: Storage,
): TagListPreferences {
  return { sort: readStoredSort(key, isTagListSort, tagListDefaults.sort, storage) };
}

/** Never throw; return the fallback for unreadable storage or a sort isSort refuses. */
export function readStoredSort<S extends string>(
  key: string,
  isSort: (value: unknown) => value is S,
  fallback: S,
  storage?: Storage,
): S {
  let raw: string | null;
  try {
    raw = (storage ?? window.localStorage).getItem(key);
  } catch {
    return fallback;
  }
  if (raw === null) return fallback;

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return fallback;
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return fallback;
  }

  const value = parsed as Record<string, unknown>;
  return isSort(value.sort) ? value.sort : fallback;
}

export function writeListSortPreferences(
  key: string,
  value: { sort: string },
  storage?: Storage,
): void {
  try {
    const stored = { sort: value.sort };
    (storage ?? window.localStorage).setItem(key, JSON.stringify(stored));
  } catch {
    // Keep the current sort even when persistence fails.
  }
}
