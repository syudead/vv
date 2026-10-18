import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

import { listTagPage, type Tag, tagPageLimit } from "../api/tags";
import { errorText, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import {
  appendUniqueTags,
  type ListRow,
  type PagedRowsQuery,
  type PageRows,
  type TagRowsQuery,
} from "./tagPageRows";

/** inconsistent means a continuation page's totalAll differs from the displayed total. */
export type MoreState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "failed"; error: UiText }
  | { kind: "inconsistent" };

export const moreIdle: MoreState = { kind: "idle" };

export type TagPageQuery = TagRowsQuery & { search: string };

export interface RowsPage<R> {
  items: R[];
  total: number;
  totalAll: number;
  nextCursor?: string;
}

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

export function useTagPage(onFirstPage: () => void) {
  return usePagedRows<Tag, TagRowsQuery>(fetchTagPage, initialTagQuery, onFirstPage);
}

/**
 * The folded query that decides reloads, so whitespace-only edits do not reload, and a ref to the
 * raw search sent with it.
 */
export function useSearchQuery(search: string) {
  const searchRef = useRef(search);
  useLayoutEffect(() => {
    searchRef.current = search;
  });
  return { normalizedQuery: foldForMatch(search).trim(), searchRef };
}

/** Share pagination/local-update rules across admin lists; retain initial fetchPage and initialQuery. */
export function usePagedRows<R extends ListRow, Q extends PagedRowsQuery>(
  fetchPage: FetchRowsPage<R, Q>,
  initialQuery: Q & { search: string },
  onFirstPage: () => void,
) {
  const fetchPageRef = useRef(fetchPage);
  const [page, setPage] = useState<PageRows<R, Q> | undefined>(undefined);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [firstPending, setFirstPending] = useState(false);
  const [more, setMore] = useState<MoreState>(moreIdle);
  /** Empty states must describe received results, not a newly typed query still awaiting its first page. */
  const [appliedSearch, setAppliedSearch] = useState("");

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

  /** First-page generations invalidate both initial and continuation responses for old conditions. */
  const generationRef = useRef(0);
  /** Mutation generations reject pending responses that could restore deleted rows or stale totals; retry the same page boundary. */
  const mutationRef = useRef(0);
  const firstAbortRef = useRef<AbortController | null>(null);
  const moreAbortRef = useRef<AbortController | null>(null);
  const queryRef = useRef<Q & { search: string }>(initialQuery);

  /** Supersede requests while retaining existing rows/counts; failures become stale-list notices when data exists. */
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

  const load = useCallback(
    (query: Q & { search: string }): Promise<void> => {
      queryRef.current = query;
      return reload();
    },
    [reload],
  );

  /** Stop pagination while loading, stale, failed, or inconsistent; a changed totalAll requires a restart instead of appending. */
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
              // Local edits may postdate this response; retry the same keyset cursor for current rows.
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

  const retryMore = useCallback(() => {
    moreRef.current = moreIdle;
    setMore(moreIdle);
    loadMore();
  }, [loadMore]);

  /** Advance mutation generation before applying local data so earlier responses are retried. */
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
    pageRef,
    load,
    reload,
    loadMore,
    retryMore,
    applyLocal,
  };
}
