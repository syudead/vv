import { Link } from "react-router";

import type { PlaylistSummary } from "../api/playlists";
import { formatDateTime, formatRelative, t } from "../i18n";
import { VideoThumbnail, VideoThumbnailDuration } from "../ui/VideoThumbnail";
import FolderArt from "../videoList/FolderArt";

/** playlistPath はプレイリストの画面の場所である。 */
export function playlistPath(id: number): string {
  return `/playlists/${String(id)}`;
}

/**
 * PlaylistCard はプレイリストの一覧の格子でプレイリストを 1 枚で出すカードである
 * （specs/044-playlists/ui-design.md「Playlists screen」）。ライブラリのグループのカード
 * （GroupCard）と同じ箱で、サムネイルの枠にはフォルダの絵柄（FolderArt）を、先頭の見せる項目の
 * サムネイル（`covers`）で描き、右下の面に項目の数を重ねる。空のプレイリストは絵柄の背板だけになる。
 * 名前の下に最後に変えた時刻を出す。カード全体がプレイリストの画面への 1 つのリンクである。
 */
export function PlaylistCard({
  playlist,
  linkRef,
}: {
  playlist: PlaylistSummary;
  /** 作った直後にフォーカスを移すためのリンクの ref。 */
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
          {/* 項目の数は、グループのカードの本数と同じ面で右下に重ねる。前に出たサムネイルより
              上に置き、絵柄の下見の操作を妨げない。 */}
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
