import { Tag as TagIcon } from "lucide-react";
import { type RefObject, useEffect, useRef, useState } from "react";

import {
  currentTags,
  refreshTags,
  revalidateTags,
  subscribeTags,
  type Tag,
} from "../api/tags";
import { compareNatural } from "../api/tagOrder";
import { t } from "../i18n";
import FilterChip from "../ui/FilterChip";
import Skeleton from "../ui/Skeleton";

export interface ActiveTagFiltersProps {
  /** 絞り込み中のタグの id（並びは問わない。表示は名前の自然順にそろえる）。 */
  tagIds: readonly number[];
  onRemove: (id: number) => void;
  /** 最後の1つを外したときのフォーカスの移し先（検索欄）。 */
  searchFieldRef: RefObject<HTMLInputElement | null>;
}

/**
 * ActiveTagFilters はライブラリの本文の先頭、要約行の上に置く「絞り込み中の
 * タグ」の行である（specs/014-video-tags/ui-design.md「Active tag filters」）。
 * タグで絞り込んでいるときだけ描画する（呼び出し側が tagIds.length === 0 で
 * 出し分ける）。
 */
export default function ActiveTagFilters({
  tagIds,
  onRemove,
  searchFieldRef,
}: ActiveTagFiltersProps) {
  const [allTags, setAllTags] = useState<Tag[] | undefined>(currentTags());
  useEffect(() => {
    let alive = true;
    revalidateTags()
      .then((loaded) => {
        if (alive) setAllTags(loaded);
      })
      .catch(() => undefined);
    const unsubscribe = subscribeTags((loaded) => {
      if (alive) setAllTags(loaded);
    });
    return () => {
      alive = false;
      unsubscribe();
    };
  }, []);

  // 共有の一覧に無いタグで絞り込んだら（使い回した一覧より後に別のタブや画面で作られた
  // タグのカードを押したときなど）、一覧を取り直して名前を引く。消えたタグで取り直しを
  // 繰り返さないよう、取り直しを求めた id は覚えておく。
  const refreshedFor = useRef(new Set<number>());
  useEffect(() => {
    if (allTags === undefined) return;
    const known = new Set(allTags.map((tag) => tag.id));
    const missing = tagIds.filter(
      (id) => !known.has(id) && !refreshedFor.current.has(id),
    );
    if (missing.length === 0) return;
    for (const id of missing) refreshedFor.current.add(id);
    void refreshTags().catch(() => undefined);
  }, [allTags, tagIds]);

  const buttonRefs = useRef(new Map<number, HTMLButtonElement>());
  const pendingFocus = useRef<number | "search" | null>(null);
  useEffect(() => {
    const pending = pendingFocus.current;
    if (pending === null) return;
    pendingFocus.current = null;
    if (pending === "search") {
      searchFieldRef.current?.focus();
      return;
    }
    buttonRefs.current.get(pending)?.focus();
  }, [tagIds, searchFieldRef]);

  const byId = new Map((allTags ?? []).map((tag) => [tag.id, tag]));
  const ordered = [...tagIds].sort((a, b) => {
    const nameA = byId.get(a)?.name;
    const nameB = byId.get(b)?.name;
    if (nameA === undefined || nameB === undefined) return a - b;
    return compareNatural(nameA, nameB) || a - b;
  });

  function remove(id: number) {
    const index = ordered.indexOf(id);
    const next = ordered[index + 1] ?? ordered[index - 1];
    if (next === undefined) {
      // 最後の1つを外すと、この行は呼び出し側で描画されなくなり、フォーカスを
      // 移す効果も走らない。外す前に検索欄へ移しておく。
      pendingFocus.current = null;
      searchFieldRef.current?.focus();
    } else {
      pendingFocus.current = next;
    }
    onRemove(id);
  }

  return (
    <div className="flex flex-wrap items-center justify-center gap-1.5">
      <span className="text-xs text-fg-muted">{t.library.activeTags.label}</span>
      <ul
        aria-label={t.library.activeTags.list}
        className="flex flex-wrap items-center gap-1.5"
      >
        {ordered.map((id) => {
          const tag = byId.get(id);
          return (
            <li key={id}>
              <FilterChip
                ref={(node) => {
                  if (node) buttonRefs.current.set(id, node);
                  else buttonRefs.current.delete(id);
                }}
                // タグの一覧をまだ取得していない間も、読み上げる名前が無くならない
                // ようにする（N2）。名前が分かれば「〈名〉の絞り込みを外す」に差し替わる。
                label={
                  tag === undefined
                    ? t.library.activeTags.remove
                    : t.library.activeTags.removeNamed(tag.name)
                }
                onRemove={() => remove(id)}
                icon={<TagIcon aria-hidden="true" />}
              >
                {tag === undefined ? (
                  <Skeleton className="h-3 w-12" />
                ) : (
                  <span title={tag.name} className="min-w-0 max-w-48 truncate">
                    {tag.name}
                  </span>
                )}
              </FilterChip>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
