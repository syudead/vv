import { useEffect, useMemo, useRef } from "react";
import { Link } from "react-router";

import { useVideos } from "../api/useVideos";
import { t } from "../i18n";
import { libraryFilterPath } from "../library/libraryPath";
import { TAG_PARAM } from "../library/tagCriteria";
import { Shelf } from "../ui/patterns/shelf";
import { Button } from "../ui/shadcn/button";
import { ShelfToolbar } from "../videoList/ShelfToolbar";
import { useShelfCriteria } from "../videoList/useShelfCriteria";
import { ShelfVideos, videoShelfStatus } from "../videoList/VideoShelf";

const text = t.tags.page;

/**
 * The tag's videos under the page's search, filter and sort in the top bar, read again in place
 * when its count or the tag's `revision` changes; the count leaves fileless videos out, so it alone
 * misses their removal.
 */
export function TagVideosShelf({
  tagId,
  videoCount,
  revision,
}: {
  tagId: number;
  videoCount: number;
  revision: number;
}) {
  const shelf = useShelfCriteria();
  const { query, favorite, sort, seed } = shelf.criteria;
  const criteria = useMemo(
    () => ({ query, favorite, sort, seed, tag: [tagId] }),
    [query, favorite, sort, seed, tagId],
  );
  const videos = useVideos(criteria, undefined, "videos");
  const { refreshInPlace } = videos;
  const read = useRef({ videoCount, revision });
  useEffect(() => {
    if (read.current.videoCount === videoCount && read.current.revision === revision)
      return;
    read.current = { videoCount, revision };
    refreshInPlace();
  }, [videoCount, revision, refreshInPlace]);

  const status = videoShelfStatus({
    videos,
    filtered: shelf.filtered,
    emptyTitle: text.notOnVideos,
    onSearch: () => shelf.toolbar.searchRef.current?.focus(),
  });
  return (
    <>
      <ShelfToolbar {...shelf.toolbar} />
      <Shelf
        title={text.videos}
        count={status === null ? t.count.videos(videos.total) : undefined}
        actions={
          <Button asChild variant="ghost" size="sm">
            <Link to={libraryFilterPath(TAG_PARAM, tagId)}>{text.openInLibrary}</Link>
          </Button>
        }
      >
        {status ?? <ShelfVideos videos={videos} />}
      </Shelf>
    </>
  );
}
