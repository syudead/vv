import { Link } from "react-router";

import type { PlaylistSummary } from "../api/playlists";
import { formatDateTime, formatRelative, t } from "../i18n";
import { VideoThumbnail, VideoThumbnailDuration } from "../ui/VideoThumbnail";
import FolderArt from "../videoList/FolderArt";

export function playlistPath(id: number): string {
  return `/playlists/${String(id)}`;
}

export function PlaylistCard({
  playlist,
  linkRef,
}: {
  playlist: PlaylistSummary;
  linkRef?: (node: HTMLAnchorElement | null) => void;
}) {
  return (
    <article
      data-playlist-id={playlist.id}
      className="group relative flex min-w-0 flex-col gap-2"
    >
      <Link
        ref={linkRef}
        to={playlistPath(playlist.id)}
        aria-label={t.playlists.rowLabel(playlist.name, playlist.itemCount)}
        className="flex min-w-0 flex-1 flex-col gap-2 rounded-md"
      >
        <VideoThumbnail>
          <FolderArt previews={playlist.covers ?? []} />
          {/* Keep the count above the front cover without intercepting preview interactions. */}
          <VideoThumbnailDuration className="pointer-events-none z-20">
            <span>{t.playlists.items(playlist.itemCount)}</span>
          </VideoThumbnailDuration>
        </VideoThumbnail>
        <div className="flex min-w-0 flex-col gap-1">
          <h3
            title={playlist.name}
            className="line-clamp-2 text-sm font-medium text-foreground break-all"
          >
            {playlist.name}
          </h3>
          <p className="text-xs text-muted-foreground tabular-nums">
            <span title={formatDateTime(playlist.updatedAt)}>
              {formatRelative(playlist.updatedAt)}
            </span>
          </p>
        </div>
      </Link>
    </article>
  );
}
