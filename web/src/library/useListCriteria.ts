import { useCallback, useEffect, useRef } from "react";
import { useLocation, useNavigate } from "react-router";

import type { VideoSort } from "../api/client";
import {
  type HistoryMode,
  type ListCriteria,
  newSeed,
  parseListCriteria,
  serializeListCriteria,
} from "./listCriteria";

export interface ListCriteriaState {
  criteria: ListCriteria;
  /** apply は条件を URL に書く。視聴状態などの個別の操作は push、入力の続きは replace。 */
  apply: (next: ListCriteria, mode: HistoryMode) => void;
}

/**
 * useListCriteria は今の URL を一覧の条件として読み、書き換える口を返す
 * （specs/013-library-search/contracts/list-url.md）。パスはそのまま残すので、
 * フォルダ画面でも同じように使える。
 *
 * preferredSort は URL に sort が無いときの並び順（端末に保存した値）である。
 */
export function useListCriteria(preferredSort: VideoSort): ListCriteriaState {
  const location = useLocation();
  const navigate = useNavigate();
  const parsed = parseListCriteria(new URLSearchParams(location.search), preferredSort);

  // sort=random で seed が無いときは、ここで作って URL に書き足す（履歴は増やさない）。
  // 書き足すまでの描画でも同じ値を使うよう、URL ごとに1つだけ作って覚える。
  const supplied = useRef<{ search: string; seed: number } | null>(null);
  let criteria = parsed.criteria;
  if (parsed.needsSeed) {
    if (supplied.current?.search !== location.search) {
      supplied.current = { search: location.search, seed: newSeed() };
    }
    criteria = { ...criteria, seed: supplied.current.seed };
  }

  const latest = useRef({ location, criteria, hasExplicitSort: parsed.hasExplicitSort });
  latest.current = { location, criteria, hasExplicitSort: parsed.hasExplicitSort };

  const write = useCallback(
    (next: ListCriteria, mode: HistoryMode) => {
      const { location: current } = latest.current;
      const search = serializeListCriteria(next).toString();
      navigate(
        { pathname: current.pathname, search: search === "" ? "" : `?${search}` },
        { replace: mode === "replace" },
      );
    },
    [navigate],
  );

  const needsSeed = parsed.needsSeed;
  useEffect(() => {
    if (needsSeed) write(latest.current.criteria, "replace");
  }, [needsSeed, location.search, write]);

  const apply = useCallback(
    (next: ListCriteria, mode: HistoryMode) => {
      const { criteria: current, hasExplicitSort } = latest.current;
      // sort の無い URL は「端末に保存した並び順」を指す。並べ替えを変えると保存も
      // 変わり、戻ったときに前の並びにならない。履歴を増やす前に、今の項目へ今の
      // 並び順を書いておく（list-url.md §3 の戻る/進む）。
      if (mode === "push" && !hasExplicitSort) write(current, "replace");
      write(next, mode);
    },
    [write],
  );

  return { criteria, apply };
}
