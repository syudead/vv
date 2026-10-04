import {
  defaultTagListSort,
  isTagListSort,
  type TagListSort,
} from "../tags/tagListOrder";

/**
 * TagListPreferences はタグ管理画面の端末の設定である。並び順だけを残し、
 * 「Tentative only」「Unused only」と検索語は残さない
 * （specs/036-tag-admin-scale/research.md R-7）。
 */
export interface TagListPreferences {
  sort: TagListSort;
}

const storageKey = "vv.tags.v1";

export const tagListDefaults: TagListPreferences = { sort: defaultTagListSort };

/**
 * readTagListPreferences はどの場合でも投げず、完全な値を返す。読めない・
 * 壊れているときは既定（名前の順）にする（`readViewPreferences` と同じ）。
 */
export function readTagListPreferences(storage?: Storage): TagListPreferences {
  let raw: string | null;
  try {
    raw = (storage ?? window.localStorage).getItem(storageKey);
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

export function writeTagListPreferences(
  value: TagListPreferences,
  storage?: Storage,
): void {
  try {
    const stored: TagListPreferences = { sort: value.sort };
    (storage ?? window.localStorage).setItem(storageKey, JSON.stringify(stored));
  } catch {
    // 保存できなくても今の画面では並び順を使える
  }
}
