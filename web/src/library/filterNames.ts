import { useEffect, useEffectEvent, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { type CreditKind, getCredit } from "../api/credits";
import {
  currentTags,
  refreshTags,
  revalidateTags,
  subscribeTags,
  type Tag,
} from "../api/tags";

// 絞り込みのチップとスマートフォルダの条件の欄が名前を引くための読み込み
// （ライブラリの「Filtering by」の行と docs/design-docs/catalog-organization.md「Conditions」）。

/** useTagNames は共有のタグの一覧を読み、絞り込み中のタグの名前を引けるようにする。 */
export function useTagNames(tagIds: readonly number[]): Map<number, Tag> {
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

  return new Map((allTags ?? []).map((tag) => [tag.id, tag]));
}

/**
 * useCreditName は人物か団体の名前を `GET /api/people/{id}`・`GET /api/organizations/{id}`
 * で読む。届くまでは undefined。`404` なら `onGone` を呼ぶ（ほかの失敗では名前の代わりの
 * Skeleton のまま、タグの一覧が読めないときのタグのチップと同じ）。
 */
export function useCreditName(
  kind: CreditKind,
  id: number,
  onGone: (kind: CreditKind) => void,
): string | undefined {
  const [loaded, setLoaded] = useState<{ kind: CreditKind; id: number; name: string }>();
  const gone = useEffectEvent(() => onGone(kind));
  useEffect(() => {
    const controller = new AbortController();
    getCredit(kind, id, controller.signal)
      .then((credit) => setLoaded({ kind, id, name: credit.name }))
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        if (failure instanceof RequestFailed && failure.status === 404) gone();
      });
    return () => controller.abort();
  }, [kind, id]);
  return loaded?.kind === kind && loaded.id === id ? loaded.name : undefined;
}
