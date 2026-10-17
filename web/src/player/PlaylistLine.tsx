import { ListVideo } from "lucide-react";
import { Link } from "react-router";

import { t } from "../i18n";
import { playlistPath } from "../playlists/playlistItems";

/**
 * PlaylistLine は題名の上に置く、プレイリスト名と何番目かの 1 行である。Group line と
 * 同じ形で、題名より小さく従の色にする。行はプレイリストの画面へのリンクである。
 * `position` が 0（今の項目が見せない項目）なら名前だけを出す。
 */
export default function PlaylistLine({
  playlistId,
  name,
  position,
  count,
  backTo,
}: {
  playlistId: number;
  name: string;
  position: number;
  count: number;
  backTo: string;
}) {
  return (
    <Link
      to={playlistPath(playlistId)}
      state={{ from: backTo }}
      aria-label={t.player.playlist.line(name, position, count)}
      data-playlist-line=""
      className="flex max-w-full min-w-0 items-center gap-1.5 self-start rounded-sm text-xs text-muted-foreground hover:text-foreground sm:text-sm"
    >
      <ListVideo className="size-4 shrink-0" aria-hidden="true" />
      <span className="truncate" title={name}>
        {name}
      </span>
      {position > 0 && (
        <>
          <span className="shrink-0" aria-hidden="true">
            ·
          </span>
          <span className="shrink-0 tabular-nums">
            {t.player.related.position(position, count)}
          </span>
        </>
      )}
    </Link>
  );
}
