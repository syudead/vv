import { Tag as TagIcon, X } from "lucide-react";
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
import { Skeleton } from "../ui/shadcn/skeleton";
import { Toggle } from "../ui/shadcn/toggle";

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
  // 開いたときの確かめ直し（`revalidateTags`）が返ったか。それより前の一覧（古い保持）で
  // 足りない id を数えると、確かめ直しの取得と並べて全件をもう一度取ってしまう。
  const [revalidated, setRevalidated] = useState(false);
  useEffect(() => {
    let alive = true;
    revalidateTags()
      .then((loaded) => {
        if (!alive) return;
        setAllTags(loaded);
        setRevalidated(true);
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

  // 確かめ直しのあとも共有の一覧に無いタグで絞り込んでいたら（使い回した一覧より後に
  // 別のタブや画面で作られたタグのカードを押したときなど）、一覧を取り直して名前を引く。消えたタグで取り直しを
  // 繰り返さないよう、取り直しを求めた id は覚えておく。
  const refreshedFor = useRef(new Set<number>());
  useEffect(() => {
    if (!revalidated || allTags === undefined) return;
    const known = new Set(allTags.map((tag) => tag.id));
    const missing = tagIds.filter(
      (id) => !known.has(id) && !refreshedFor.current.has(id),
    );
    if (missing.length === 0) return;
    for (const id of missing) refreshedFor.current.add(id);
    void refreshTags().catch(() => undefined);
  }, [allTags, revalidated, tagIds]);

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
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs text-muted-foreground">{t.library.activeTags.label}</span>
      <ul
        aria-label={t.library.activeTags.list}
        className="flex flex-wrap items-center gap-2"
      >
        {ordered.map((id) => {
          const tag = byId.get(id);
          // タグの一覧をまだ取得していない間も、読み上げる名前が無くならない
          // ようにする（N2）。名前が分かれば「〈名〉の絞り込みを外す」に差し替わる。
          const label =
            tag === undefined
              ? t.library.activeTags.remove
              : t.library.activeTags.removeNamed(tag.name);
          return (
            <li key={id}>
              {/* 効いている絞り込みは押された Toggle で、押し戻すと外れる
                  （web/registry/rules/components.md「Toggle and ToggleGroup」）。 */}
              <Toggle
                ref={(node) => {
                  if (node) buttonRefs.current.set(id, node);
                  else buttonRefs.current.delete(id);
                }}
                variant="outline"
                size="sm"
                pressed
                onPressedChange={() => remove(id)}
                aria-label={label}
                title={label}
              >
                <TagIcon aria-hidden="true" />
                {tag === undefined ? (
                  <Skeleton className="h-3 w-12" />
                ) : (
                  <span title={tag.name} className="max-w-chip-label min-w-0 truncate">
                    {tag.name}
                  </span>
                )}
                <X aria-hidden="true" />
              </Toggle>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
