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

/** ExtraValues は画面固有のパラメータの、名前ごとの値（`getAll` の形）である。 */
export type ExtraValues = Readonly<Record<string, readonly string[]>>;

export interface ListCriteriaState {
  criteria: ListCriteria;
  /**
   * apply は条件を URL に書く。視聴状態などの個別の操作は push、入力の続きは replace。
   *
   * `extra` に `extraParams` のパラメータの名前と値を渡すと、この呼び出しでその値を
   * 差し替える。渡さなかったパラメータは、今の URL にある値（まだ描画していない直前の
   * apply があれば、それが書いた値）をそのまま残す。
   */
  apply: (next: ListCriteria, mode: HistoryMode, extra?: ExtraValues) => void;
}

const noExtraParams: readonly string[] = [];

/**
 * useListCriteria は今の URL を一覧の条件として読み、書き換える口を返す
 * （specs/013-library-search/contracts/list-url.md）。パスはそのまま残すので、
 * フォルダ画面でも同じように使える。
 *
 * preferredSort は URL に sort が無いときの並び順（端末に保存した値）である。
 *
 * `extraParams` は、画面の固有のもの（ライブラリのタグ絞り込み `tag`、人物・団体の
 * `person`・`organization`）を運ぶ口である。中身は解釈せず、この経路が書き換える URL の
 * すべての経路（apply・seed の補完・sort を確定する置き換え）で値を残す。省略する
 * 画面（フォルダ画面）では、それらのパラメータには一切触れない。呼び手は参照の
 * 変わらない配列を渡す。
 *
 * ゲストとして描くときは、所有者のデータに依る条件（視聴状態・お気に入りのみ・
 * 最近再生した順・お気に入りにした日時・`extraParams` のタグ・人物・団体）を既定に
 * 丸めて返す。URL に残っていたら履歴を増やさずに URL も直す。端末に保存した並び順
 * （preferredSort）が所有者だけの並び順のときも丸めるが、
 * 保存値そのものは書き換えない（specs/016-single-account-auth/ui-design.md
 * 「Guest degradation」、contracts/guest-api.md §3）。
 */
export function useListCriteria(
  preferredSort: VideoSort,
  extraParams: readonly string[] = noExtraParams,
): ListCriteriaState {
  const location = useLocation();
  const navigate = useNavigate();
  const guest = useAudience() === "guest";
  const searchParams = new URLSearchParams(location.search);
  const parsed = parseListCriteria(searchParams, preferredSort);
  const rawExtra: ExtraValues = Object.fromEntries(
    extraParams.map((name) => [name, searchParams.getAll(name)]),
  );
  // ゲストの URL に残った、使えない条件。丸めた条件で URL を書き直す。
  const guestLeftovers =
    guest &&
    (parsed.criteria.watch !== "all" ||
      parsed.criteria.favorite ||
      (parsed.hasExplicitSort && ownerOnlySort(parsed.criteria.sort)) ||
      Object.values(rawExtra).some((values) => values.length > 0));
  const extra: ExtraValues = guest ? {} : rawExtra;

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
    (next: ListCriteria, mode: HistoryMode, extraValues?: ExtraValues) => {
      const { location: current } = latest.current;
      const params = serializeListCriteria(next);
      const written: Record<string, readonly string[]> = {};
      for (const name of extraParams) {
        const values = extraValues?.[name] ?? latest.current.extra[name] ?? [];
        written[name] = values;
        for (const value of values) params.append(name, value);
      }
      // 次の描画より前にもう一度書く呼び出し（人物と団体の名前の読みが同時に `404`
      // など）が、今書いた値の上に積むよう覚える。描画すると URL の値で上書きされる。
      latest.current = { ...latest.current, extra: written };
      const search = params.toString();
      navigate(
        { pathname: current.pathname, search: search === "" ? "" : `?${search}` },
        { replace: mode === "replace" },
      );
    },
    [navigate, extraParams],
  );

  const needsSeed = parsed.needsSeed;
  useEffect(() => {
    if (needsSeed || guestLeftovers) write(latest.current.criteria, "replace");
  }, [needsSeed, guestLeftovers, location.search, write]);

  const apply = useCallback(
    (next: ListCriteria, mode: HistoryMode, extraValues?: ExtraValues) => {
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
