import { Ban, CircleAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { errorText, t, type UiText } from "../i18n";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageSection } from "../ui/patterns/page-section";
import { SettingsPage } from "../ui/patterns/settings-page";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { TagsSubpageHeader } from "./TagsSubpageHeader";
import { useRejectedNames } from "./useRejectedNames";

/** `/tags/rejected`: the names automatic tagging skips, loaded in pages, each allowed again in place. */
export default function RejectedNamesPage() {
  const rejected = useRejectedNames();
  const { page, error, morePending, moreError, loadMore } = rejected;
  const names = page?.items;
  const hasMore = page?.nextCursor !== undefined;
  const titleRef = useRef<HTMLHeadingElement | null>(null);
  const [sentinel, setSentinel] = useState<HTMLElement | null>(null);
  const loadMoreRef = useRef(loadMore);
  loadMoreRef.current = loadMore;
  const loadedCount = names?.length ?? 0;

  // Reobserve after every page so a still-visible last row loads another page.
  // Refreshes ignore sentinel events; reobserve even when the row count is unchanged.
  useEffect(() => {
    if (sentinel === null || !hasMore || morePending || moreError !== null) return;
    if (typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMoreRef.current();
      },
      { rootMargin: "0px 0px 240px 0px" },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [sentinel, hasMore, morePending, moreError, loadedCount, rejected.epoch]);

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
      await rejected.forget(name);
      const index = order.indexOf(name);
      const rest = order.filter((item) => item !== name);
      const next = rest[Math.min(Math.max(index, 0), rest.length - 1)];
      setTimeout(() => {
        const target = next === undefined ? undefined : removeButtons.current.get(next);
        (target ?? titleRef.current)?.focus();
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
    <SettingsPage
      header={
        <TagsSubpageHeader
          title={t.tags.rejectedNames.heading}
          description={t.tags.rejectedNames.description}
          titleRef={titleRef}
        />
      }
    >
      <PageSection title={t.tags.rejectedNames.section}>
        {names === undefined && error === null && (
          <LoadingState label={t.tags.loading} layout="table" count={4} />
        )}
        {names === undefined && error !== null && (
          <ErrorState
            title={t.tags.rejectedNames.loadFailed}
            retryLabel={t.common.retry}
            onRetry={rejected.reload}
          />
        )}
        {/* An empty loaded page may still have more results; keep pagination until the cursor is exhausted. */}
        {names !== undefined && names.length === 0 && !hasMore && (
          <EmptyState
            icon={<Ban aria-hidden="true" />}
            title={t.tags.rejectedNames.empty}
          />
        )}
        {names?.map((name, index) => (
          <div
            key={name}
            ref={index === names.length - 1 ? setSentinel : undefined}
            title={name}
            data-rejected-name=""
            className="flex items-center gap-3"
          >
            <span className="min-w-0 flex-1 truncate font-medium">{name}</span>
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
          </div>
        ))}
        {/* With every loaded name removed, this row stands in for the last one. */}
        {names?.length === 0 && hasMore && (
          <div ref={setSentinel} aria-hidden="true" className="h-px" />
        )}
        {morePending && <LoadMoreRow status="loading" label={t.common.loading} />}
        {moreError !== null && (
          <LoadMoreRow
            status="failed"
            title={t.tags.rejectedNames.loadMoreFailed}
            retryLabel={t.common.retry}
            onRetry={loadMore}
          />
        )}
        {removeError !== null && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{removeError}</AlertTitle>
          </Alert>
        )}
      </PageSection>
    </SettingsPage>
  );
}
