import { useCallback, useRef } from "react";
import { useLocation, useNavigate } from "react-router";

import { type HistoryMode, normalizeQuery } from "../videoList/listCriteria";
import { isTagListSort, type TagListSort } from "./tagListOrder";

/** TagListTab はタグ管理画面のタブである（「Tags」「Rejected names」「Kinds」）。 */
export type TagListTab = "tags" | "rejected" | "kinds";

/**
 * TagKindFilter はタグの一覧の種類の絞り込みの値である。種類の id（正の整数の文字列）か、
 * 種類の無いタグの `none`。null は絞り込まない。
 */
export type TagKindFilter = string | null;

/** parseTagListTab は URL の `tab` を読む。知らない値は「Tags」。 */
function parseTagListTab(value: string | null): TagListTab {
  return value === "rejected" || value === "kinds" ? value : "tags";
}

/** parseTagKindFilter は URL の `kind` を読む。正の整数と `none` のほかは絞り込まない。 */
function parseTagKindFilter(value: string | null): TagKindFilter {
  if (value === null) return null;
  return value === "none" || /^[1-9]\d*$/.test(value) ? value : null;
}

/**
 * TagListCriteria はタグ管理画面の URL に載せる条件である。ライブラリの一覧の条件
 * と同じく、再読み込み・戻る・進むで
 * 検索語・絞り込み・並び順・タブが残る。
 */
export interface TagListCriteria {
  query: string;
  tentativeOnly: boolean;
  unusedOnly: boolean;
  /** 種類の絞り込み（`kind`）。 */
  kind: TagKindFilter;
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
      kind: parseTagKindFilter(params.get("kind")),
      sort: hasExplicitSort ? rawSort : preferredSort,
      tab: parseTagListTab(params.get("tab")),
    },
    hasExplicitSort,
  };
}

/**
 * serializeTagListCriteria は条件を URL のクエリにする。偽の絞り込みと「Tags」の
 * タブは書かない。sort は書く — 省略すると「端末に保存した並び順」の意味になり、
 * 並び順を変えたあとに戻るで前の並びに戻れなくなる。
 * 順は q・tentative・unused・kind・sort・tab で固定する。
 */
export function serializeTagListCriteria(criteria: TagListCriteria): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  if (criteria.tentativeOnly) params.set("tentative", "1");
  if (criteria.unusedOnly) params.set("unused", "1");
  if (criteria.kind !== null) params.set("kind", criteria.kind);
  params.set("sort", criteria.sort);
  if (criteria.tab !== "tags") params.set("tab", criteria.tab);
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
  return useUrlCriteria(
    (params) => parseTagListCriteria(params, preferredSort),
    serializeTagListCriteria,
  );
}

/**
 * useUrlCriteria は `useTagListCriteria` の、URL の読み書き（`parse`・`serialize`）を受け取る
 * 形である。人物・団体の管理画面（`web/src/credits/`）も同じ履歴の規則で `q` と `sort` を
 * 載せる。`serialize` は最初に受けたものを使い続ける。
 */
export function useUrlCriteria<C>(
  parse: (params: URLSearchParams) => { criteria: C; hasExplicitSort: boolean },
  serialize: (criteria: C) => URLSearchParams,
): {
  criteria: C;
  apply: (change: Partial<C>, mode: HistoryMode) => void;
} {
  const location = useLocation();
  const navigate = useNavigate();
  const { criteria, hasExplicitSort } = parse(new URLSearchParams(location.search));
  const latest = useRef({ location, criteria, hasExplicitSort });
  // 描き直す前に続けて呼ばれても、同じ URL の項目を重ねて積まないよう、書いたばかりの
  // クエリを覚える。位置が変わった描画で忘れる。
  const written = useRef<string | null>(null);
  if (latest.current.location !== location) written.current = null;
  latest.current = { location, criteria, hasExplicitSort };
  const serializeRef = useRef(serialize);

  const apply = useCallback(
    (change: Partial<C>, mode: HistoryMode) => {
      const {
        location: current,
        criteria: now,
        hasExplicitSort: explicit,
      } = latest.current;
      const write = (next: C, how: HistoryMode) => {
        const search = serializeRef.current(next).toString();
        const target = search === "" ? "" : `?${search}`;
        // 履歴に積むのは今の URL と違うときだけ。Radix のタブは1回の押下で値の変更を
        // 2度通すことがあり、そのまま積むと「戻る」が同じ画面に留まる。
        if (how === "push" && target === (written.current ?? current.search)) return;
        written.current = target;
        navigate(
          { pathname: current.pathname, search: target },
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
