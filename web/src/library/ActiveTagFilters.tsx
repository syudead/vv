import { Tag as TagIcon, X } from "lucide-react";
import { type RefObject, useEffect, useRef, useState } from "react";

import { currentTags, refreshTags, subscribeTags, type Tag } from "../api/tags";
import { compareNatural } from "../api/tagOrder";
import { cn } from "../lib/cn";
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
    refreshTags()
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
    pendingFocus.current = next ?? "search";
    onRemove(id);
  }

  return (
    <div className="flex flex-wrap items-center justify-center gap-1.5">
      <span className="text-xs text-fg-muted">タグで絞り込み中</span>
      <ul aria-label="絞り込み中のタグ" className="flex flex-wrap items-center gap-1.5">
        {ordered.map((id) => {
          const tag = byId.get(id);
          return (
            <li key={id}>
              <button
                ref={(node) => {
                  if (node) buttonRefs.current.set(id, node);
                  else buttonRefs.current.delete(id);
                }}
                type="button"
                // タグの一覧をまだ取得していない間も、読み上げる名前が無くならない
                // ようにする（N2）。名前が分かれば「〈名〉の絞り込みを外す」に差し替わる。
                aria-label={
                  tag === undefined
                    ? "タグの絞り込みを外す"
                    : `${tag.name}の絞り込みを外す`
                }
                onClick={() => remove(id)}
                className={cn(
                  "flex h-6 items-center gap-1 rounded-sm bg-accent-soft px-1.5 text-xs text-link",
                  "hover:ring-1 hover:ring-inset hover:ring-border-strong",
                )}
              >
                <TagIcon aria-hidden="true" className="size-3 shrink-0" />
                {tag === undefined ? (
                  <Skeleton className="h-3 w-12" />
                ) : (
                  <span title={tag.name} className="min-w-0 max-w-48 truncate">
                    {tag.name}
                  </span>
                )}
                <X aria-hidden="true" className="size-3 shrink-0" />
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
