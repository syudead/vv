import { ListVideo, Plus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { type PlaylistList, listPlaylists, type PlaylistSummary } from "../api/playlists";
import { errorText, t } from "../i18n";
import { CardGrid } from "../ui/patterns/card-grid";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import NewPlaylistDialog from "./NewPlaylistDialog";
import { PlaylistCard } from "./PlaylistCard";

type More = { kind: "idle" } | { kind: "loading" } | { kind: "failed"; error: unknown };

type Page =
  | { kind: "loading" }
  | { kind: "failed" }
  | {
      kind: "ready";
      items: readonly PlaylistSummary[];
      /** Total count is independent of loaded pages. */
      total: number;
      nextCursor?: string;
      /** A mismatching next-page version means the order changed. */
      version: string;
      /** Stale pages require replacement from page one. */
      stale: boolean;
      more: More;
    };

function readyPage(list: PlaylistList): Page {
  return {
    kind: "ready",
    items: list.items,
    total: list.total,
    nextCursor: list.nextCursor,
    version: list.version,
    stale: false,
    more: { kind: "idle" },
  };
}

/**
 * Restart from page one on version changes to avoid missing playlists moved before the cursor.
 * Keep cards on refresh failure and make Retry restart the first page; prepend creations
 * optimistically before refreshing.
 */
export default function PlaylistsPage() {
  const [page, setPage] = useState<Page>({ kind: "loading" });
  const [creating, setCreating] = useState(false);
  const [focusId, setFocusId] = useState<number | null>(null);
  const links = useRef(new Map<number, HTMLAnchorElement>());
  const pageRef = useRef(page);
  // Allow one list request at a time; new reads and post-create refreshes abort the previous request.
  const inflight = useRef<AbortController | null>(null);

  const showPage = useCallback((next: Page) => {
    pageRef.current = next;
    setPage(next);
  }, []);

  useEffect(() => {
    const previous = document.title;
    document.title = t.playlists.documentTitle;
    return () => {
      document.title = previous;
    };
  }, []);

  const begin = useCallback(() => {
    inflight.current?.abort();
    const controller = new AbortController();
    inflight.current = controller;
    return controller;
  }, []);

  const load = useCallback(() => {
    const controller = begin();
    listPlaylists({ signal: controller.signal }).then(
      (list) => {
        if (controller.signal.aborted) return;
        inflight.current = null;
        showPage(readyPage(list));
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        inflight.current = null;
        showPage({ kind: "failed" });
      },
    );
  }, [begin, showPage]);

  useEffect(() => {
    load();
    return () => {
      inflight.current?.abort();
    };
  }, [load]);

  /**
   * Replace from page one when stale, otherwise append the next page; no-op while loading or
   * exhausted.
   */
  const loadMore = useCallback(() => {
    const read = (cursor: string | undefined) => {
      const controller = begin();
      listPlaylists({ cursor, signal: controller.signal }).then(
        (list) => {
          const now = pageRef.current;
          if (controller.signal.aborted || now.kind !== "ready") return;
          if (cursor === undefined) {
            inflight.current = null;
            showPage(readyPage(list));
            return;
          }
          if (list.version !== now.version) {
            // The order changed during pagination; restart to avoid missing moved playlists.
            showPage({ ...now, stale: true, more: { kind: "loading" } });
            read(undefined);
            return;
          }
          inflight.current = null;
          showPage({
            ...now,
            items: [...now.items, ...list.items],
            total: list.total,
            nextCursor: list.nextCursor,
            more: { kind: "idle" },
          });
        },
        (error: unknown) => {
          const now = pageRef.current;
          if (isAborted(error) || controller.signal.aborted || now.kind !== "ready")
            return;
          inflight.current = null;
          showPage({ ...now, more: { kind: "failed", error } });
        },
      );
    };

    const current = pageRef.current;
    if (current.kind !== "ready" || current.more.kind === "loading") return;
    if (!current.stale && current.nextCursor === undefined) return;
    showPage({ ...current, more: { kind: "loading" } });
    read(current.stale ? undefined : current.nextCursor);
  }, [begin, showPage]);

  const sentinel = useRef<HTMLDivElement | null>(null);
  const canLoadMore =
    page.kind === "ready" &&
    page.more.kind === "idle" &&
    (page.stale || page.nextCursor !== undefined);
  const nextCursor = page.kind === "ready" ? page.nextCursor : undefined;
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !canLoadMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
    // Reattach observation after success or failure so a still-visible sentinel can load more.
  }, [canLoadMore, loadMore, nextCursor]);

  // Focus the new card after the dialog finishes restoring its trigger's focus.
  useEffect(() => {
    if (focusId === null) return;
    const timer = setTimeout(() => {
      links.current.get(focusId)?.focus();
      setFocusId(null);
    }, 0);
    return () => clearTimeout(timer);
  }, [focusId]);

  function created(playlist: PlaylistSummary) {
    const current = pageRef.current;
    const rest =
      current.kind === "ready"
        ? current.items.filter((item) => item.id !== playlist.id)
        : [];
    // Prepend optimistically while replacing stale pages with a fresh first page.
    showPage({
      kind: "ready",
      items: [playlist, ...rest],
      total: (current.kind === "ready" ? current.total : 0) + 1,
      nextCursor: current.kind === "ready" ? current.nextCursor : undefined,
      version: current.kind === "ready" ? current.version : "",
      stale: true,
      more: { kind: "idle" },
    });
    setCreating(false);
    setFocusId(playlist.id);
    loadMore();
  }

  function retry() {
    showPage({ kind: "loading" });
    load();
  }

  const empty = page.kind === "ready" && page.items.length === 0;
  const newButton = (
    <Button size="sm" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" />
      {t.playlists.new}
    </Button>
  );

  return (
    <ListPage
      header={
        <PageHeader
          title={t.playlists.title}
          count={
            page.kind === "ready" && !empty ? t.playlists.count(page.total) : undefined
          }
          // In the empty state, expose only one primary creation action.
          actions={empty ? undefined : newButton}
        />
      }
    >
      {page.kind === "loading" && <LoadingState label={t.list.loading} layout="grid" />}
      {page.kind === "failed" && (
        <ErrorState
          title={t.playlists.loadFailed}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      )}
      {empty && (
        <EmptyState
          icon={<ListVideo aria-hidden="true" />}
          title={t.playlists.empty.title}
          description={t.playlists.empty.description}
          action={newButton}
        />
      )}
      {page.kind === "ready" && !empty && (
        <CardGrid>
          {page.items.map((playlist) => (
            <PlaylistCard
              key={playlist.id}
              playlist={playlist}
              linkRef={(node) => {
                if (node === null) links.current.delete(playlist.id);
                else links.current.set(playlist.id, node);
              }}
            />
          ))}
        </CardGrid>
      )}
      {page.kind === "ready" && page.more.kind === "loading" && (
        <LoadMoreRow status="loading" label={t.list.loading} />
      )}
      {page.kind === "ready" && page.more.kind === "failed" && (
        <LoadMoreRow
          status="failed"
          title={
            page.stale
              ? t.playlists.loadFailed
              : t.list.loadMoreFailed(errorText(page.more.error))
          }
          retryLabel={t.common.retry}
          onRetry={loadMore}
        />
      )}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
      {creating && (
        <NewPlaylistDialog onClose={() => setCreating(false)} onCreated={created} />
      )}
    </ListPage>
  );
}
