import { useCallback, useRef } from "react";
import { useLocation, useNavigate } from "react-router";

import { type HistoryMode, normalizeQuery } from "../videoList/listCriteria";
import { isTagListSort, type TagListSort } from "./tagListOrder";

/** TagListTab はタグ管理画面のタブである（「Tags」と「Rejected names」）。 */
export type TagListTab = "tags" | "rejected";

/**
 * TagListCriteria はタグ管理画面の URL に載せる条件である
 * （specs/036-tag-admin-scale/ui-design.md「URL state」）。ライブラリの一覧の条件
 * （specs/013-library-search/contracts/list-url.md）と同じく、再読み込み・戻る・進むで
 * 検索語・絞り込み・並び順・タブが残る。
 */
export interface TagListCriteria {
  query: string;
  tentativeOnly: boolean;
  unusedOnly: boolean;
  sort: TagListSort;
  tab: TagListTab;
}

/**
 * parseTagListCriteria は URL のクエリを条件にする。解釈できない値は既定として扱い、
 * 誤りを出さない。sort が無いか解釈できないときは、端末に保存した並び順
 * （preferredSort）を使う（ライブラリの `parseListCriteria` と同じ）。
 */
export function parseTagListCriteria(
  params: URLSearchParams,
  preferredSort: TagListSort,
): { criteria: TagListCriteria; hasExplicitSort: boolean } {
  const rawSort = params.get("sort");
  const hasExplicitSort = isTagListSort(rawSort);
  return {
    criteria: {
      query: normalizeQuery(params.get("q") ?? ""),
      tentativeOnly: params.get("tentative") === "1",
      unusedOnly: params.get("unused") === "1",
      sort: hasExplicitSort ? rawSort : preferredSort,
      tab: params.get("tab") === "rejected" ? "rejected" : "tags",
    },
    hasExplicitSort,
  };
}

/**
 * serializeTagListCriteria は条件を URL のクエリにする。偽の絞り込みと「Tags」の
 * タブは書かない。sort は書く — 省略すると「端末に保存した並び順」の意味になり、
 * 並び順を変えたあとに戻るで前の並びに戻れなくなる（list-url.md §1 と同じ）。
 * 順は q・tentative・unused・sort・tab で固定する。
 */
export function serializeTagListCriteria(criteria: TagListCriteria): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  if (criteria.tentativeOnly) params.set("tentative", "1");
  if (criteria.unusedOnly) params.set("unused", "1");
  params.set("sort", criteria.sort);
  if (criteria.tab === "rejected") params.set("tab", "rejected");
  return params;
}

/**
 * useTagListCriteria は今の URL をタグ管理画面の条件として読み、書き換える口を返す。
 * 書き換えは今の条件に `change` を重ねたもので、個別の操作は push、検索の入力の
 * 続きは replace（`SearchBox` の `HistoryMode`）。
 */
export function useTagListCriteria(preferredSort: TagListSort): {
  criteria: TagListCriteria;
  apply: (change: Partial<TagListCriteria>, mode: HistoryMode) => void;
} {
  const location = useLocation();
  const navigate = useNavigate();
  const { criteria, hasExplicitSort } = parseTagListCriteria(
    new URLSearchParams(location.search),
    preferredSort,
  );
  const latest = useRef({ location, criteria, hasExplicitSort });
  latest.current = { location, criteria, hasExplicitSort };

  const apply = useCallback(
    (change: Partial<TagListCriteria>, mode: HistoryMode) => {
      const {
        location: current,
        criteria: now,
        hasExplicitSort: explicit,
      } = latest.current;
      const write = (next: TagListCriteria, how: HistoryMode) => {
        const search = serializeTagListCriteria(next).toString();
        navigate(
          { pathname: current.pathname, search: search === "" ? "" : `?${search}` },
          { replace: how === "replace" },
        );
      };
      // sort の無い URL は「端末に保存した並び順」を指す。履歴を増やす前に、今の項目へ
      // 今の並び順を書いておく（ライブラリの `useListCriteria` と同じ）。
      if (mode === "push" && !explicit) write(now, "replace");
      write({ ...now, ...change }, mode);
    },
    [navigate],
  );

  return { criteria, apply };
}
