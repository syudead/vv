import { Film } from "lucide-react";
import { type ReactNode, useCallback } from "react";
import { useLocation } from "react-router";

import type { VideosState } from "../api/useVideos";
import { t, type UiText } from "../i18n";
import { CardGrid } from "../ui/patterns/card-grid";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { useLoadMoreSentinel } from "./pageHooks";
import { NoMatches } from "./states";
import VideoCard from "./VideoCard";

// The video shelves of the tag, person and organization pages; see docs/design-docs/library-ui.md, Video shelves.

const text = t.list.shelf;

const noPreviewReset = () => undefined;

/** Six placeholder cards while a shelf's first page loads. */
export function ShelfLoading() {
  return <LoadingState label={t.list.loading} layout="grid" size={0} count={6} />;
}

/** The failed first read of a shelf, with `Retry`. */
export function ShelfFailed({ error, onRetry }: { error: UiText; onRetry: () => void }) {
  return (
    <ErrorState
      title={text.failed(error)}
      retryLabel={t.common.retry}
      onRetry={onRetry}
    />
  );
}

/**
 * The block drawn in place of the cards while the first read is pending or failed, or when it
 * found nothing; null once there are cards. Nothing found under a search or filter is a no-match
 * whose button focuses the search, so it never reads as a subject without videos.
 */
export function videoShelfStatus({
  videos,
  filtered,
  emptyTitle,
  onSearch,
}: {
  videos: VideosState;
  filtered: boolean;
  emptyTitle: UiText;
  onSearch: () => void;
}): ReactNode {
  if (videos.loading) return <ShelfLoading />;
  if (videos.items.length > 0) return null;
  if (videos.error !== null)
    return <ShelfFailed error={videos.error} onRetry={videos.reload} />;
  if (filtered) return <NoMatches onSearch={onSearch} />;
  return <EmptyState icon={<Film aria-hidden="true" />} title={emptyTitle} />;
}

/** Every video read so far as cards that wrap, reading the next page as the end comes into view. */
export function ShelfVideos({ videos }: { videos: VideosState }) {
  const location = useLocation();
  const backTo = `${location.pathname}${location.search}`;
  const { items, loading, loadingMore, error, hasMore, loadMore, retryLoadMore } = videos;
  const more = useCallback(() => loadMore(), [loadMore]);
  const sentinel = useLoadMoreSentinel(
    hasMore && !loading && !loadingMore && error === null,
    more,
    noPreviewReset,
  );
  return (
    <>
      <CardGrid size={0}>
        {items.map((item) =>
          item.kind === "video" ? (
            <VideoCard
              key={item.video.id}
              video={item.video}
              backTo={backTo}
              selected={false}
              selectionMode={false}
            />
          ) : null,
        )}
      </CardGrid>
      {loadingMore && <LoadMoreRow status="loading" label={text.loadingMore} />}
      {error !== null && (
        <LoadMoreRow
          status="failed"
          title={text.moreFailed}
          description={error}
          retryLabel={t.common.retry}
          onRetry={retryLoadMore}
        />
      )}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </>
  );
}
