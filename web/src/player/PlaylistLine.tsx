import { ListVideo } from "lucide-react";
import { Link } from "react-router";

import { t } from "../i18n";
import { playlistPath } from "../playlists/playlistItems";

/** Position zero identifies a hidden item; show only the playlist name. */
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
