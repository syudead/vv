import { useEffect, useLayoutEffect, useRef } from "react";

import type { TagKind } from "../api/tagKinds";
import type { TagKindFilter } from "./tagListUrl";

/**
 * kindNameOf は種類の絞り込みの種類の名前である。`none`・絞り込み無し・まだ読んでいない
 * 種類は undefined。
 */
export function kindNameOf(
  kinds: readonly TagKind[] | undefined,
  kind: TagKindFilter | undefined,
): string | undefined {
  if (kind === undefined || kind === null || kind === "none") return undefined;
  return kinds?.find((item) => String(item.id) === kind)?.name;
}

/**
 * isGoneKind は、URL の `kind` の種類が、読めた種類の一覧に無いかである。一覧を画面を
 * 開いてから読めていない（`fresh` が false）間と読み込みの失敗の間は、外さない
 * （specs/051-tag-kinds/ui-design.md「Kind filter」）。
 */
export function isGoneKind(
  kind: TagKindFilter,
  kinds: readonly TagKind[] | undefined,
  fresh: boolean,
): boolean {
  if (!fresh || kinds === undefined || kind === null || kind === "none") return false;
  return !kinds.some((item) => String(item.id) === kind);
}

/**
 * useTagKindFilter はタグ管理画面の種類の絞り込みの、もう無い種類の扱いである。URL の
 * `kind` の種類が読めた一覧に無ければ `onGone` を呼ぶ（呼び手は `replace` で URL から外し、
 * トーストを出す。条件が変わるので一覧は読み直される）。チップに出す名前を返す。
 */
export function useTagKindFilter({
  kind,
  kinds,
  fresh,
  onGone,
}: {
  kind: TagKindFilter;
  kinds: readonly TagKind[] | undefined;
  fresh: boolean;
  onGone: () => void;
}): { kindName: string | undefined } {
  const onGoneRef = useRef(onGone);
  useLayoutEffect(() => {
    onGoneRef.current = onGone;
  });
  const gone = isGoneKind(kind, kinds, fresh);
  useEffect(() => {
    if (gone) onGoneRef.current();
  }, [gone, kind]);
  return { kindName: kindNameOf(kinds, kind) };
}
