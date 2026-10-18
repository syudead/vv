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

export interface ScanIssueList {
  scanId: number | null;
  items: ScanIssue[];
  nextCursor: string | undefined;
  loadingMore: boolean;
  /** Retain the last successful list on failure. */
  error: UiText | null;
  loadMore: () => void;
}

interface Loaded {
  scanId: number | null;
  items: ScanIssue[];
  nextCursor: string | undefined;
}

const empty: Loaded = { scanId: null, items: [], nextCursor: undefined };

/** Limit same-revision retries when response scanId differs from current Scan.id. */
const MISMATCH_RETRIES = 2;

/** Reload through the existing count on same-scan revisions to retain rows and scroll. */
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
    // The scan changed mid-read; restart at the new scan's first page.
    if (page === null || page.scanId !== first.scanId) return page;
    items.push(...page.items);
    cursor = page.nextCursor;
  }
  return { scanId: first.scanId, items, nextCursor: cursor };
}

/**
 * Issues are not streamed: reload on scan ID or revision changes, preserving the loaded count
 * within one scan. Mismatched IDs refresh scan status; failures retain rows and retry on status
 * refresh even at the same revision.
 */
export function useScanIssues(scan: Scan | null, refreshScan: () => void): ScanIssueList {
  const [loaded, setLoaded] = useState<Loaded>(empty);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [retry, setRetry] = useState(0);
  const loadedRef = useRef(loaded);
  loadedRef.current = loaded;
  // Allow only one refresh or pagination request at a time.
  const controller = useRef<AbortController | null>(null);
  const mismatches = useRef(0);
  const refreshRef = useRef(refreshScan);
  refreshRef.current = refreshScan;
  const errorRef = useRef(error);
  errorRef.current = error;

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
          // Discard issues for a different scan and refresh status first.
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

  // After a failure, successful status refresh retries issues even if the revision is unchanged.
  useEffect(() => {
    if (scan === null || errorRef.current === null || controller.current !== null) return;
    setRetry((value) => value + 1);
  }, [scan]);

  const loadMore = useCallback(() => {
    const current = loadedRef.current;
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
