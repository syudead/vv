import { useCallback, useEffect, useEffectEvent, useRef, useState } from "react";

import {
  currentTagKinds,
  getTagKinds,
  refreshTagKinds,
  subscribeTagKinds,
  type TagKind,
} from "../api/tagKinds";
import { t } from "../i18n";
import { useToast } from "../ui/Toast";
import type { HistoryMode } from "../videoList/listCriteria";
import { withoutTagKind, withTagKind } from "./creditCriteria";

/** ApplyTagKind は今の条件のまま `tagKind` だけを書き換える、呼び手の `apply` である。 */
export type ApplyTagKind = (
  extra: Readonly<Record<string, string[]>>,
  mode: HistoryMode,
) => void;

export interface LibraryTagKinds {
  /** 共有の種類の一覧。まだ読めていなければ undefined。 */
  kinds: TagKind[] | undefined;
  /** 絞り込み中の種類の名前。一覧が届く前や種類が無いときは undefined。 */
  name: string | undefined;
  /** 種類の一覧を読む（ポップオーバーを開いたとき。保持があれば要求しない）。 */
  load: () => void;
  /** 種類を選ぶ（undefined は `Any kind`）。履歴を 1 つ増やす。 */
  change: (id: number | undefined) => void;
}

const ignore = () => undefined;

/**
 * useLibraryTagKinds はライブラリのタグの種類の絞り込み（URL の `tagKind`）の、種類の一覧と
 * 書き換えを受け持つ。
 *
 * - 一覧は `api/tagKinds.ts` の共有の保持から読む。`tagKind` が効いていて保持が無ければ
 *   すぐに、そうでなければポップオーバーを開いたとき（`load`）に `GET /api/tag-kinds` を
 *   送る。両者は `getTagKinds` で 1 回の要求を共有する。
 * - 読めた一覧に `tagKind` の種類が無ければ、`replace` で URL から外してトーストを出す。
 *   ほかの絞り込みは残る。この画面を開く前からあった保持で見つからないときは、別の
 *   タブで作られた種類かもしれないので、取り直してから決める。
 * - 読み込みや取り直しに失敗しても何も外さない（チップは名前の代わりの Skeleton のまま）。
 *   取り直しに失敗したときは、ポップオーバーを開いたときにもう一度取り直す。
 */
export function useLibraryTagKinds(
  tagKind: number | undefined,
  apply: ApplyTagKind,
): LibraryTagKinds {
  const toast = useToast();
  const [kinds, setKinds] = useState<TagKind[] | undefined>(currentTagKinds);
  // 開いた時点の保持（これと同じ配列で見つからないときは、古いかもしれない）。
  const [cachedAtMount] = useState<TagKind[] | undefined>(currentTagKinds);

  useEffect(() => subscribeTagKinds(setKinds), []);

  // 開いた時点の保持のままで `tagKind` が見つからない間は、その一覧は古いかもしれない。
  const unconfirmed =
    tagKind !== undefined &&
    kinds !== undefined &&
    kinds === cachedAtMount &&
    !kinds.some((kind) => kind.id === tagKind);

  // 古いかもしれない一覧のままなら、`getTagKinds` は同じ保持を返すだけなので取り直す
  // （先の取り直しが失敗していても、ポップオーバーを開けば再び試す）。
  const load = useCallback(() => {
    if (unconfirmed) refreshTagKinds().catch(ignore);
    else getTagKinds().then(setKinds).catch(ignore);
  }, [unconfirmed]);

  const active = tagKind !== undefined;
  const missingList = kinds === undefined;
  useEffect(() => {
    if (active && missingList) load();
  }, [active, missingList, load]);

  const gone = useEffectEvent(() => {
    toast(t.library.activeTags.kindGone);
    apply(withoutTagKind, "replace");
  });
  // 同じ id で 2 度取り直したり外したりしない（StrictMode の二度の実行や、外す前の再描画）。
  // `tagKind` が外れたら忘れ、同じ画面で同じ id に戻ったときにもう一度確かめる。
  const handled = useRef<number | undefined>(undefined);
  const refreshed = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (tagKind === undefined) {
      handled.current = undefined;
      refreshed.current = undefined;
      return;
    }
    if (kinds === undefined || kinds.some((kind) => kind.id === tagKind)) return;
    // 古いかもしれない一覧では外さない。取り直しに失敗したら、外さずにそのまま待つ。
    if (kinds === cachedAtMount) {
      if (refreshed.current === tagKind) return;
      refreshed.current = tagKind;
      refreshTagKinds().catch(ignore);
      return;
    }
    if (handled.current === tagKind) return;
    handled.current = tagKind;
    gone();
  }, [cachedAtMount, kinds, tagKind]);

  const change = useCallback(
    (id: number | undefined) =>
      apply(id === undefined ? withoutTagKind : withTagKind(id), "push"),
    [apply],
  );

  const name =
    tagKind === undefined ? undefined : kinds?.find((kind) => kind.id === tagKind)?.name;
  return { kinds, name, load, change };
}
