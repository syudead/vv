import { useCallback, useEffect, useRef, useState } from "react";

import {
  isAborted,
  listCurrentScanIssues,
  MAX_SCAN_ISSUE_PAGE_SIZE,
  SCAN_ISSUE_PAGE_SIZE,
  type Scan,
  type ScanIssue,
  type ScanIssuePage,
} from "../api/client";
import { errorText, type UiText } from "../i18n";

/** ScanIssueList は設定の「Videos with problems」に出す、読めた分の一覧である。 */
export interface ScanIssueList {
  /** 読めた一覧がどの取り込みのものか。まだ何も読めていなければ null。 */
  scanId: number | null;
  items: ScanIssue[];
  /** 続きがあるときだけ入る。 */
  nextCursor: string | undefined;
  /** 続きを読んでいるあいだ true。 */
  loadingMore: boolean;
  /** 直近の読み込みの失敗。読めた一覧はそのまま残す。 */
  error: UiText | null;
  loadMore: () => void;
}

interface Loaded {
  scanId: number | null;
  items: ScanIssue[];
  nextCursor: string | undefined;
}

const empty: Loaded = { scanId: null, items: [], nextCursor: undefined };

/** 応答の scanId が今の Scan.id と違ったときに、同じ版で読み直す回数の上限。 */
const MISMATCH_RETRIES = 2;

/**
 * loadPages は一覧を先頭から、`count` 件に届くか続きが無くなるまで読む。同じ取り込みの
 * 中で版が変わったとき、「Show more」で読んだ分まで読み直して行とスクロールを保つ。
 */
async function loadPages(
  count: number,
  signal: AbortSignal,
): Promise<ScanIssuePage | null> {
  const first = await listCurrentScanIssues({
    limit: Math.min(MAX_SCAN_ISSUE_PAGE_SIZE, Math.max(SCAN_ISSUE_PAGE_SIZE, count)),
    signal,
  });
  if (first === null) return null;
  const items = [...first.items];
  let cursor = first.nextCursor;
  while (cursor !== undefined && items.length < count) {
    const page = await listCurrentScanIssues({
      limit: Math.min(
        MAX_SCAN_ISSUE_PAGE_SIZE,
        Math.max(SCAN_ISSUE_PAGE_SIZE, count - items.length),
      ),
      cursor,
      signal,
    });
    // 読んでいる途中で取り込みが替わった。新しい方の1ページ目から読み直させる。
    if (page === null || page.scanId !== first.scanId) return page;
    items.push(...page.items);
    cursor = page.nextCursor;
  }
  return { scanId: first.scanId, items, nextCursor: cursor };
}

/**
 * useScanIssues は直近の取り込みの問題の一覧を読む
 * （specs/024-import-progress/contracts/scan-api.md §3・§4）。
 *
 * - 一覧は SSE では届かない。`Scan.id` か `Scan.issues.revision` が変わったら読み直す。
 *   つながり直したあとの取り直しで版が変わっていれば、切れていた間の問題も反映される。
 * - 新しい取り込みでは先頭から読み直す。同じ取り込みの中では、読めた件数まで読み直す。
 * - 応答の `scanId` が今の `Scan.id` と違えば、状態を取り直し（`refreshScan`）、読み直す。
 * - 読めなかったときは、最後に読めた一覧を残す（ui-design.md「States」）。
 */
export function useScanIssues(scan: Scan | null, refreshScan: () => void): ScanIssueList {
  const [loaded, setLoaded] = useState<Loaded>(empty);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [retry, setRetry] = useState(0);
  const loadedRef = useRef(loaded);
  loadedRef.current = loaded;
  // 走っている読み込み（読み直しか続き）。同時に1つだけにする。
  const controller = useRef<AbortController | null>(null);
  const mismatches = useRef(0);
  const refreshRef = useRef(refreshScan);
  refreshRef.current = refreshScan;

  const scanId = scan?.id ?? null;
  const revision = scan?.issues.revision ?? null;
  const hasIssues = scan !== null && scan.issues.failed + scan.issues.substituted > 0;

  useEffect(() => {
    mismatches.current = 0;
  }, [scanId, revision]);

  useEffect(() => {
    controller.current?.abort();
    controller.current = null;
    setLoadingMore(false);
    if (scanId === null || !hasIssues) {
      setLoaded(empty);
      setError(null);
      return;
    }
    const current = loadedRef.current;
    const sameScan = current.scanId === scanId;
    if (!sameScan && current.scanId !== null) setLoaded(empty);
    const count = sameScan
      ? Math.max(SCAN_ISSUE_PAGE_SIZE, current.items.length)
      : SCAN_ISSUE_PAGE_SIZE;
    const abort = new AbortController();
    controller.current = abort;
    const settle = () => {
      if (controller.current === abort) controller.current = null;
    };
    loadPages(count, abort.signal)
      .then((page) => {
        if (abort.signal.aborted) return;
        settle();
        if (page === null) {
          setLoaded(empty);
          setError(null);
          return;
        }
        if (page.scanId !== scanId) {
          // 今の状態より新しい（か古い）取り込みの一覧である。捨てて、状態から取り直す。
          if (mismatches.current < MISMATCH_RETRIES) {
            mismatches.current += 1;
            refreshRef.current();
            setRetry((value) => value + 1);
          }
          return;
        }
        setLoaded({
          scanId: page.scanId,
          items: page.items,
          nextCursor: page.nextCursor,
        });
        setError(null);
      })
      .catch((reason: unknown) => {
        if (abort.signal.aborted || isAborted(reason)) return;
        settle();
        setError(errorText(reason));
      });
    return () => abort.abort();
  }, [scanId, revision, hasIssues, retry]);

  const loadMore = useCallback(() => {
    const current = loadedRef.current;
    // 読み直しや続きの読み込みの途中では重ねない。
    if (current.nextCursor === undefined || controller.current !== null) return;
    const abort = new AbortController();
    controller.current = abort;
    const settle = () => {
      if (controller.current === abort) controller.current = null;
      setLoadingMore(false);
    };
    setLoadingMore(true);
    listCurrentScanIssues({ cursor: current.nextCursor, signal: abort.signal })
      .then((page) => {
        if (abort.signal.aborted) return;
        settle();
        if (page === null || page.scanId !== current.scanId) {
          // 取り込みが替わった。状態を取り直し、先頭から読み直す。
          refreshRef.current();
          setRetry((value) => value + 1);
          return;
        }
        setLoaded({
          scanId: current.scanId,
          items: [...current.items, ...page.items],
          nextCursor: page.nextCursor,
        });
        setError(null);
      })
      .catch((reason: unknown) => {
        if (abort.signal.aborted || isAborted(reason)) return;
        settle();
        setError(errorText(reason));
      });
  }, []);

  useEffect(() => () => controller.current?.abort(), []);

  return { ...loaded, loadingMore, error, loadMore };
}
