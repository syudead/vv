import { useCallback, useEffect, useRef } from "react";
import { useLocation, useNavigate } from "react-router";

import type { VideoSort } from "../api/client";
import { useAudience } from "../auth/audience";
import {
  guestListCriteria,
  type HistoryMode,
  type ListCriteria,
  newSeed,
  ownerOnlySort,
  parseListCriteria,
  serializeListCriteria,
} from "./listCriteria";

export interface ListCriteriaState {
  criteria: ListCriteria;
  /**
   * apply は条件を URL に書く。視聴状態などの個別の操作は push、入力の続きは replace。
   *
   * `extra` を渡すと、`extraParam` で指定した画面固有のパラメータの値をこの呼び出しで
   * 差し替える。省略すると、今の URL にある値をそのまま残す。
   */
  apply: (next: ListCriteria, mode: HistoryMode, extra?: readonly string[]) => void;
}

/**
 * useListCriteria は今の URL を一覧の条件として読み、書き換える口を返す
 * （specs/013-library-search/contracts/list-url.md）。パスはそのまま残すので、
 * フォルダ画面でも同じように使える。
 *
 * preferredSort は URL に sort が無いときの並び順（端末に保存した値）である。
 *
 * `extraParam` は、画面の固有のもの（ライブラリのタグ絞り込み `tag` など）を運ぶ口
 * （Plan の Structural Decisions 15）。中身は解釈せず、この経路が書き換える URL の
 * すべての経路（apply・seed の補完・sort を確定する置き換え）で値を残す。省略する
 * 画面（フォルダ画面）では、そのパラメータには一切触れない。
 *
 * ゲストとして描くときは、所有者のデータに依る条件（視聴状態・最近再生した順・
 * `extraParam` のタグ）を既定に丸めて返す。URL に残っていたら履歴を増やさずに
 * URL も直す。端末に保存した並び順（preferredSort）が最近再生した順のときも丸めるが、
 * 保存値そのものは書き換えない（specs/016-single-account-auth/ui-design.md
 * 「Guest degradation」、contracts/guest-api.md §3）。
 */
export function useListCriteria(
  preferredSort: VideoSort,
  extraParam?: string,
): ListCriteriaState {
  const location = useLocation();
  const navigate = useNavigate();
  const guest = useAudience() === "guest";
  const searchParams = new URLSearchParams(location.search);
  const parsed = parseListCriteria(searchParams, preferredSort);
  const rawExtra = extraParam === undefined ? [] : searchParams.getAll(extraParam);
  // ゲストの URL に残った、使えない条件。丸めた条件で URL を書き直す。
  const guestLeftovers =
    guest &&
    (parsed.criteria.watch !== "all" ||
      (parsed.hasExplicitSort && ownerOnlySort(parsed.criteria.sort)) ||
      rawExtra.length > 0);
  const extra = guest ? [] : rawExtra;

  // sort=random で seed が無いときは、ここで作って URL に書き足す（履歴は増やさない）。
  // 書き足すまでの描画でも同じ値を使うよう、URL ごとに1つだけ作って覚える。
  const supplied = useRef<{ search: string; seed: number } | null>(null);
  let criteria = guest ? guestListCriteria(parsed.criteria) : parsed.criteria;
  if (parsed.needsSeed) {
    if (supplied.current?.search !== location.search) {
      supplied.current = { search: location.search, seed: newSeed() };
    }
    criteria = { ...criteria, seed: supplied.current.seed };
  }

  const latest = useRef({
    location,
    criteria,
    hasExplicitSort: parsed.hasExplicitSort,
    extra,
  });
  latest.current = { location, criteria, hasExplicitSort: parsed.hasExplicitSort, extra };

  const write = useCallback(
    (next: ListCriteria, mode: HistoryMode, extraValues?: readonly string[]) => {
      const { location: current } = latest.current;
      const params = serializeListCriteria(next);
      if (extraParam !== undefined) {
        for (const value of extraValues ?? latest.current.extra) {
          params.append(extraParam, value);
        }
      }
      const search = params.toString();
      navigate(
        { pathname: current.pathname, search: search === "" ? "" : `?${search}` },
        { replace: mode === "replace" },
      );
    },
    [navigate, extraParam],
  );

  const needsSeed = parsed.needsSeed;
  useEffect(() => {
    if (needsSeed || guestLeftovers) write(latest.current.criteria, "replace");
  }, [needsSeed, guestLeftovers, location.search, write]);

  const apply = useCallback(
    (next: ListCriteria, mode: HistoryMode, extraValues?: readonly string[]) => {
      const { criteria: current, hasExplicitSort } = latest.current;
      // sort の無い URL は「端末に保存した並び順」を指す。並べ替えを変えると保存も
      // 変わり、戻ったときに前の並びにならない。履歴を増やす前に、今の項目へ今の
      // 並び順を書いておく（list-url.md §3 の戻る/進む）。今の extra の値も残す。
      if (mode === "push" && !hasExplicitSort) write(current, "replace");
      write(next, mode, extraValues);
    },
    [write],
  );

  return { criteria, apply };
}
