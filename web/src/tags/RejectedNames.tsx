import { Ban, CircleAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import type { RejectedTagNameList } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { DataTable } from "../ui/patterns/data-table";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { TableBody, TableCell, TableRow } from "../ui/shadcn/table";

export default function RejectedNames({
  page,
  error,
  onRetry,
  morePending,
  moreError,
  onLoadMore,
  resetKey,
  onForget,
  onFocusFallback,
}: {
  page: RejectedTagNameList | undefined;
  error: UiText | null;
  onRetry: () => void;
  morePending: boolean;
  moreError: UiText | null;
  onLoadMore: () => void;
  /** Changes on each first-page response to reobserve the sentinel. */
  resetKey: number;
  onForget: (name: string) => Promise<void>;
  /** Focus the Rejected names tab after removing the final row. */
  onFocusFallback: () => void;
}) {
  const names = page?.items;
  const hasMore = page?.nextCursor !== undefined;
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const onLoadMoreRef = useRef(onLoadMore);
  onLoadMoreRef.current = onLoadMore;
  const loadedCount = names?.length ?? 0;

  // Reobserve after every page so a still-visible sentinel loads another page.
  // Refreshes ignore sentinel events; reobserve even when the row count is unchanged.
  useEffect(() => {
    const target = sentinelRef.current;
    if (target === null || !hasMore || morePending || moreError !== null) return;
    if (typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onLoadMoreRef.current();
      },
      { rootMargin: "0px 0px 240px 0px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, morePending, moreError, loadedCount, resetKey]);

  const [removing, setRemoving] = useState<ReadonlySet<string>>(new Set());
  const [removeError, setRemoveError] = useState<UiText | null>(null);
  const removeButtons = useRef(new Map<string, HTMLButtonElement>());
  const removingRef = useRef(new Set<string>());

  async function forget(name: string) {
    if (removingRef.current.has(name) || names === undefined) return;
    const order = [...names];
    removingRef.current.add(name);
    setRemoving(new Set(removingRef.current));
    setRemoveError(null);
    try {
      await onForget(name);
      const index = order.indexOf(name);
      const rest = order.filter((item) => item !== name);
      const next = rest[Math.min(Math.max(index, 0), rest.length - 1)];
      setTimeout(() => {
        const target = next === undefined ? undefined : removeButtons.current.get(next);
        if (target === undefined) onFocusFallback();
        else target.focus();
      }, 0);
    } catch (failure) {
      setRemoveError(t.tags.rejectedNames.removeFailed(name, errorText(failure)));
      // Restore the same button after failure because disabling it lost focus.
      setTimeout(() => removeButtons.current.get(name)?.focus(), 0);
    } finally {
      removingRef.current.delete(name);
      setRemoving(new Set(removingRef.current));
    }
  }

  return (
    <>
      {names === undefined && error === null && (
        <LoadingState label={t.tags.loading} layout="table" count={4} />
      )}
      {names === undefined && error !== null && (
        <ErrorState
          title={t.tags.rejectedNames.loadFailed}
          retryLabel={t.common.retry}
          onRetry={onRetry}
        />
      )}
      {/* An empty loaded page may still have more results; keep pagination until the cursor is exhausted. */}
      {names !== undefined && names.length === 0 && !hasMore && (
        <EmptyState
          icon={<Ban aria-hidden="true" />}
          title={t.tags.rejectedNames.empty}
        />
      )}
      {names !== undefined && (names.length > 0 || hasMore) && (
        <>
          <p className="text-sm text-muted-foreground">
            {t.tags.rejectedNames.description}
          </p>
          <DataTable label={t.tags.rejectedNames.heading}>
            <TableBody aria-busy={morePending || undefined}>
              {names.map((name) => (
                <TableRow key={name} title={name}>
                  <TableCell className="w-full max-w-0">
                    <span className="block truncate font-medium">{name}</span>
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      ref={(node) => {
                        if (node) removeButtons.current.set(name, node);
                        else removeButtons.current.delete(name);
                      }}
                      variant="outline"
                      size="sm"
                      aria-label={t.tags.rejectedNames.allow(name)}
                      disabled={removing.has(name)}
                      onClick={() => void forget(name)}
                    >
                      {t.tags.rejectedNames.allowShort}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </DataTable>
          {morePending && <LoadMoreRow status="loading" label={t.common.loading} />}
          {moreError !== null && (
            <LoadMoreRow
              status="failed"
              title={t.tags.rejectedNames.loadMoreFailed}
              retryLabel={t.common.retry}
              onRetry={onLoadMore}
            />
          )}
          {hasMore && <div ref={sentinelRef} aria-hidden="true" className="h-px" />}
        </>
      )}
      {removeError !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{removeError}</AlertTitle>
        </Alert>
      )}
    </>
  );
}
