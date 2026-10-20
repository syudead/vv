import { Film } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { useLocation } from "react-router";

import type { components } from "../../api/gen/openapi";
import { useVideos, type VideosState } from "../../api/useVideos";
import { t } from "../../i18n";
import { CardGrid } from "../../ui/patterns/card-grid";
import { EmptyState } from "../../ui/patterns/empty-state";
import { ErrorState } from "../../ui/patterns/error-state";
import { LoadMoreRow } from "../../ui/patterns/load-more-row";
import { LoadingState } from "../../ui/patterns/loading-state";
import { PageHeader } from "../../ui/patterns/page-header";
import { Shelf, ShelfItem } from "../../ui/patterns/shelf";
import { Button } from "../../ui/shadcn/button";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";
import { DEFAULT_SORT } from "../../videoList/listCriteria";
import { useLoadMoreSentinel } from "../../videoList/pageHooks";
import VideoCard from "../../videoList/VideoCard";

type Role = components["schemas"]["WorkPersonRole"];

const roles: readonly Role[] = ["performer", "director", "photographer", "streamer"];

const text = t.credits.personPage;

function isRole(value: string): value is Role {
  return (roles as readonly string[]).includes(value);
}

/** The cards a shelf shows before See all. */
const shelfSize = 12;

/** The person's videos in the library's default order, narrowed to one role on the server. */
export function usePersonVideos(personId: number) {
  const [role, setRole] = useState<Role | null>(null);
  const criteria = useMemo(
    () => ({ sort: DEFAULT_SORT, person: personId, personRole: role ?? undefined }),
    [personId, role],
  );
  return { role, setRole, videos: useVideos(criteria, undefined, "videos") };
}

export function PersonVideosHeader({ videos }: { videos: VideosState }) {
  return (
    <PageHeader
      title={text.videos}
      headingLevel={2}
      count={videos.loading ? undefined : t.credits.row.videoCount(videos.total)}
    />
  );
}

export function RoleFilter({
  role,
  onChange,
}: {
  role: Role | null;
  onChange: (role: Role | null) => void;
}) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      aria-label={text.roleFilter}
      value={role ?? "all"}
      // Pressing the pressed item again keeps it pressed; All is the way back.
      onValueChange={(value) => {
        if (value === "") return;
        onChange(isRole(value) ? value : null);
      }}
    >
      <ToggleGroupItem value="all" className="flex-none px-3">
        {text.roles.all}
      </ToggleGroupItem>
      {roles.map((item) => (
        <ToggleGroupItem key={item} value={item} className="flex-none px-3">
          {text.roles[item]}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

const noPreviewReset = () => undefined;

export function PersonVideosBody({
  videos,
  role,
}: {
  videos: VideosState;
  role: Role | null;
}) {
  const location = useLocation();
  const backTo = `${location.pathname}${location.search}`;
  const { items, loading, loadingMore, error, hasMore, loadMore, retryLoadMore, reload } =
    videos;
  const more = useCallback(() => loadMore(), [loadMore]);
  const sentinel = useLoadMoreSentinel(
    hasMore && !loading && !loadingMore && error === null,
    more,
    noPreviewReset,
  );

  if (loading) {
    return <LoadingState label={text.loading} layout="grid" size={0} count={6} />;
  }
  if (error !== null && items.length === 0) {
    return (
      <ErrorState
        title={text.videosFailed(error)}
        retryLabel={t.common.retry}
        onRetry={reload}
      />
    );
  }
  if (items.length === 0) {
    return (
      <EmptyState
        icon={<Film aria-hidden="true" />}
        title={role === null ? text.noVideos : text.noVideosAs(text.roles[role])}
      />
    );
  }
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

/**
 * One shelf per role the person has videos in, in the roles' order. `all` is the person's videos in
 * any role: its states stand for the shelves, and with none the empty state shows instead.
 */
export function PersonShelves({
  personId,
  all,
  onSeeAll,
}: {
  personId: number;
  all: VideosState;
  onSeeAll: (role: Role) => void;
}) {
  if (all.loading) {
    return <LoadingState label={text.loading} layout="grid" size={0} count={6} />;
  }
  if (all.error !== null && all.items.length === 0) {
    return (
      <ErrorState
        title={text.videosFailed(all.error)}
        retryLabel={t.common.retry}
        onRetry={all.reload}
      />
    );
  }
  if (all.items.length === 0) {
    return <EmptyState icon={<Film aria-hidden="true" />} title={text.noVideos} />;
  }
  return (
    <>
      {roles.map((role) => (
        <RoleShelf key={role} personId={personId} role={role} onSeeAll={onSeeAll} />
      ))}
    </>
  );
}

/** A role without videos, or whose read failed, draws nothing; the other shelves still show. */
function RoleShelf({
  personId,
  role,
  onSeeAll,
}: {
  personId: number;
  role: Role;
  onSeeAll: (role: Role) => void;
}) {
  const location = useLocation();
  const backTo = `${location.pathname}${location.search}`;
  const criteria = useMemo(
    () => ({ sort: DEFAULT_SORT, person: personId, personRole: role }),
    [personId, role],
  );
  const videos = useVideos(criteria, undefined, "videos");
  if (videos.loading || videos.items.length === 0) return null;
  const name = text.roles[role];
  return (
    <Shelf
      title={name}
      count={t.credits.row.videoCount(videos.total)}
      action={
        <Button
          variant="ghost"
          size="sm"
          aria-label={text.seeAllAs(name)}
          onClick={() => onSeeAll(role)}
        >
          {text.seeAll}
        </Button>
      }
    >
      {videos.items.slice(0, shelfSize).map((item) =>
        item.kind === "video" ? (
          <ShelfItem key={item.video.id}>
            <VideoCard
              video={item.video}
              backTo={backTo}
              selected={false}
              selectionMode={false}
            />
          </ShelfItem>
        ) : null,
      )}
    </Shelf>
  );
}
