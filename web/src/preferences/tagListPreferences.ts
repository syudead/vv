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
  let raw: string | null;
  try {
    raw = (storage ?? window.localStorage).getItem(key);
  } catch {
    return { ...tagListDefaults };
  }
  if (raw === null) return { ...tagListDefaults };

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { ...tagListDefaults };
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return { ...tagListDefaults };
  }

  const value = parsed as Record<string, unknown>;
  return { sort: isTagListSort(value.sort) ? value.sort : tagListDefaults.sort };
}

export function writeListSortPreferences(
  key: string,
  value: TagListPreferences,
  storage?: Storage,
): void {
  try {
    const stored: TagListPreferences = { sort: value.sort };
    (storage ?? window.localStorage).setItem(key, JSON.stringify(stored));
  } catch {
    // Keep the current sort even when persistence fails.
  }
}
