import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

import { listTagPage, type Tag, tagPageLimit } from "../api/tags";
import { errorText, type UiText } from "../i18n";
import {
  appendUniqueTags,
  type ListRow,
  type ListRowsQuery,
  type PageRows,
  type TagRowsQuery,
} from "./tagPageRows";

/**
 * MoreState は一覧の末尾の続きの状態である。同時に 1 つだけで、`inconsistent` は続きの応答の `totalAll` が
 * 画面の値と違ったとき。
 */
export type MoreState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "failed"; error: UiText }
  | { kind: "inconsistent" };

export const moreIdle: MoreState = { kind: "idle" };

/** TagPageQuery は読む条件である。`search` は打った検索語、`query` はその照合形。 */
export type TagPageQuery = TagRowsQuery & { search: string };

/** RowsPage はサーバーが返す 1 ページである（`GET /api/tags`・`/api/people` などの応答）。 */
export interface RowsPage<R> {
  items: R[];
  total: number;
  totalAll: number;
  nextCursor?: string;
}

/**
 * FetchRowsPage は 1 ページを読む。`query` は行の条件、`search` は打った検索語（空でない
 * 照合形のときにサーバーへ送る語）、`cursor` は続きのときだけ渡す。
 */
export type FetchRowsPage<R, Q> = (
  query: Q,
  search: string,
  cursor: string | undefined,
  signal: AbortSignal,
) => Promise<RowsPage<R>>;

const initialTagQuery: TagPageQuery = {
  query: "",
  search: "",
  tentativeOnly: false,
  unusedOnly: false,
  kind: null,
  sort: "name",
};

function fetchTagPage(
  query: TagRowsQuery,
  search: string,
  cursor: string | undefined,
  signal: AbortSignal,
): Promise<RowsPage<Tag>> {
  return listTagPage(
    {
      q: query.query === "" ? undefined : search.trim(),
      tentative: query.tentativeOnly,
      unused: query.unusedOnly,
      kind: query.kind ?? undefined,
      sort: query.sort,
      cursor,
      limit: tagPageLimit,
    },
    signal,
  );
}

/**
 * useTagPage はタグ管理画面の一覧を、条件ごとに `GET /api/tags` から 100 件ずつ読む。操作の結果は
 * 一覧を取り直さずに `applyLocal` で読み込んだ行へ反映する（R-12、`tagPageRows.ts`）。
 * `onFirstPage` は先頭のページを受けて差し替えたあとに呼ぶ。
 */
export function useTagPage(onFirstPage: () => void) {
  return usePagedRows<Tag, TagRowsQuery>(fetchTagPage, initialTagQuery, onFirstPage);
}

/**
 * usePagedRows は `useTagPage` の、読み方（`fetchPage`）と最初の条件を受け取る形である。
 * 人物・団体の管理画面（`web/src/credits/`）も同じ先頭・続き・局所の反映の規則で読む。
 * `fetchPage` と `initialQuery` は最初に受けたものを使い続ける。
 */
export function usePagedRows<R extends ListRow, Q extends ListRowsQuery>(
  fetchPage: FetchRowsPage<R, Q>,
  initialQuery: Q & { search: string },
  onFirstPage: () => void,
) {
  const fetchPageRef = useRef(fetchPage);
  /** page は読み込んだ行と件数である。先頭のページをまだ一度も受けていなければ undefined。 */
  const [page, setPage] = useState<PageRows<R, Q> | undefined>(undefined);
  /**
   * loadError は先頭のページの失敗である。一覧を持っていなければ失敗の表示、
   * 持っていれば帯の中の「Stale list」の箱になる。
   */
  const [loadError, setLoadError] = useState<UiText | null>(null);
  /** firstPending は先頭のページを待っているかである。 */
  const [firstPending, setFirstPending] = useState(false);
  const [more, setMore] = useState<MoreState>(moreIdle);
  /**
   * appliedSearch は、今の行を読んだときの検索の入力である。空の状態の文言はこれを
   * 出す。新しい検索の先頭のページを待つ間は前の結果が残るので、打ち直した入力を
   * 出すと、まだ読んでいない語に「一致が無い」と言ってしまう。
   */
  const [appliedSearch, setAppliedSearch] = useState("");

  /**
   * pageRef・moreRef・firstPendingRef・loadErrorRef は、要求の応答のあとや
   * 続きのきっかけで今の状態を読むための控えである。描くたびに差し替える。
   */
  const pageRef = useRef<PageRows<R, Q> | undefined>(page);
  const moreRef = useRef<MoreState>(more);
  const firstPendingRef = useRef(firstPending);
  const loadErrorRef = useRef<UiText | null>(loadError);
  const onFirstPageRef = useRef(onFirstPage);
  useLayoutEffect(() => {
    pageRef.current = page;
    moreRef.current = more;
    firstPendingRef.current = firstPending;
    loadErrorRef.current = loadError;
    onFirstPageRef.current = onFirstPage;
  });

  /**
   * generationRef は先頭のページの要求の通し番号である。条件を変えるたびに進め、
   * 古い条件の応答（先頭のページも続きも）を捨てる。
   */
  const generationRef = useRef(0);
  /**
   * mutationRef は、操作の結果を読み込んだ行へ局所で反映した回数である（`applyLocal`）。
   * 要求を送ったあとに反映があれば、その応答は反映した変更の前の一覧かもしれないので、
   * 行と件数に入れずに同じ条件で読み直す（先頭のページは先頭から、続きは同じカーソルで）。
   * 入れると、消したタグが戻る・件数が操作の前に戻るなど、局所の反映が巻き戻る。
   */
  const mutationRef = useRef(0);
  const firstAbortRef = useRef<AbortController | null>(null);
  const moreAbortRef = useRef<AbortController | null>(null);
  /** queryRef は最後に読み直しを始めた条件である。「Retry」「Reload」が同じ条件で読む。 */
  const queryRef = useRef<Q & { search: string }>(initialQuery);

  /**
   * reload は今の条件（`queryRef`）で先頭のページを読み直す。進行中の要求
   * （先頭のページと続き）を打ち切り、世代を進めて古い応答を捨てる。届くまで前の
   * 行と件数を残し（`Skeleton` に戻さない）、届いたら差し替えて `onFirstPage` を呼ぶ。
   * 失敗したら、一覧を持っていなければ失敗の表示、持っていれば「Stale list」の箱に
   * する。
   */
  const reload = useCallback((): Promise<void> => {
    const run = (): Promise<void> => {
      firstAbortRef.current?.abort();
      moreAbortRef.current?.abort();
      generationRef.current += 1;
      const generation = generationRef.current;
      const controller = new AbortController();
      firstAbortRef.current = controller;
      const { search: searched, ...rest } = queryRef.current;
      const query = rest as unknown as Q;
      const mutation = mutationRef.current;
      firstPendingRef.current = true;
      setFirstPending(true);
      moreRef.current = moreIdle;
      setMore(moreIdle);
      return fetchPageRef.current(query, searched, undefined, controller.signal).then(
        (result) => {
          if (generation !== generationRef.current) return;
          // 送ったあとに局所の反映があった。応答はその変更を映していないかもしれない。
          if (mutation !== mutationRef.current) return run();
          setAppliedSearch(searched);
          setPage({
            rows: result.items,
            total: result.total,
            totalAll: result.totalAll,
            nextCursor: result.nextCursor,
            boundary: result.nextCursor === undefined ? undefined : result.items.at(-1),
            query,
          });
          setLoadError(null);
          setFirstPending(false);
          onFirstPageRef.current();
        },
        (failure: unknown) => {
          if (generation !== generationRef.current) return;
          setFirstPending(false);
          setLoadError(errorText(failure));
        },
      );
    };
    return run();
  }, []);

  /** load は条件を変えて先頭のページを読み直す。 */
  const load = useCallback(
    (query: Q & { search: string }): Promise<void> => {
      queryRef.current = query;
      return reload();
    },
    [reload],
  );

  /**
   * loadMore は続きの 1 ページを `nextCursor` で読み、`id` の重複を捨てて末尾に足す。
   * 続きを読んでいる間・失敗や食い違いを出している間・先頭のページを待つ間・
   * 「Stale list」の間は読まない（同時に 1 つだけ。持っているカーソルが前の条件の
   * ものかもしれない）。応答の `totalAll` が画面の値と違えば、行は残して続きを止め、
   * 「一覧が変わった」を出す。
   */
  const loadMore = useCallback(() => {
    const run = () => {
      const current = pageRef.current;
      if (current?.nextCursor === undefined) return;
      if (moreRef.current.kind !== "idle") return;
      if (firstPendingRef.current || loadErrorRef.current !== null) return;
      const generation = generationRef.current;
      const mutation = mutationRef.current;
      const controller = new AbortController();
      moreAbortRef.current = controller;
      moreRef.current = { kind: "loading" };
      setMore(moreRef.current);
      fetchPageRef
        .current(
          current.query,
          queryRef.current.search,
          current.nextCursor,
          controller.signal,
        )
        .then(
          (result) => {
            if (generation !== generationRef.current) return;
            if (mutation !== mutationRef.current) {
              // 送ったあとに局所の反映があった。応答の行と件数はその変更の前かもしれない
              // ので捨て、同じカーソルで読み直す（keyset なので、境より後ろの今の行が返る）。
              moreRef.current = moreIdle;
              setMore(moreIdle);
              run();
              return;
            }
            if (result.totalAll !== pageRef.current?.totalAll) {
              moreRef.current = { kind: "inconsistent" };
              setMore(moreRef.current);
              return;
            }
            setPage((latest) =>
              latest === undefined
                ? latest
                : {
                    ...latest,
                    rows: appendUniqueTags(latest.rows, result.items),
                    total: result.total,
                    nextCursor: result.nextCursor,
                    boundary:
                      result.nextCursor === undefined ? undefined : result.items.at(-1),
                  },
            );
            moreRef.current = moreIdle;
            setMore(moreIdle);
          },
          (failure: unknown) => {
            if (generation !== generationRef.current) return;
            moreRef.current = { kind: "failed", error: errorText(failure) };
            setMore(moreRef.current);
          },
        );
    };
    run();
  }, []);

  /** retryMore は続きの失敗の「Retry」で、同じカーソルで読み直す。 */
  const retryMore = useCallback(() => {
    moreRef.current = moreIdle;
    setMore(moreIdle);
    loadMore();
  }, [loadMore]);

  /**
   * applyLocal は、操作の結果を読み込んだ行と件数へ局所で反映する。反映の前に送った
   * 先頭のページ・続きの応答を捨てて読み直させるため、`mutationRef` を進める。
   */
  const applyLocal = useCallback(
    (update: (current: PageRows<R, Q> | undefined) => PageRows<R, Q> | undefined) => {
      mutationRef.current += 1;
      setPage(update);
    },
    [],
  );

  useEffect(
    () => () => {
      firstAbortRef.current?.abort();
      moreAbortRef.current?.abort();
    },
    [],
  );

  return {
    page,
    loadError,
    firstPending,
    more,
    appliedSearch,
    /** pageRef は応答のあとで今の行と条件を読むための控えである。 */
    pageRef,
    load,
    reload,
    loadMore,
    retryMore,
    applyLocal,
  };
}
