import { type ReactNode, useState } from "react";

import type { Video } from "../api/client";
import AddToGroupButton from "../groups/AddToGroupButton";
import { t, type UiText } from "../i18n";
import { isRemoteId } from "../lib/remoteId";
import AddToPlaylistButton from "../playlists/AddToPlaylistButton";
import FavoriteToggle from "../ui/FavoriteToggle";
import { FailureLine } from "./tabs/panelParts";

/**
 * The title at the column's full width, then the owner's actions on the video as a whole on a row
 * of their own. Callers key by video ID so a failure does not survive navigation.
 */
export default function TitleRow({
  video,
  title,
  onFavorite,
  actions,
}: {
  video: Video;
  title: ReactNode;
  /** The owner's further actions, after the three on the video as a whole. */
  actions?: ReactNode;
  /** Resolve after toggling and refreshing the video; reject on failure. Absent for guests. */
  onFavorite: ((favorite: boolean) => Promise<void>) | undefined;
}) {
  const [failure, setFailure] = useState<UiText | null>(null);
  if (onFavorite === undefined) return title;
  const favorite = video.favorite === true;
  // Playlists stay on each node; the paired VVMDM's video keeps its favorite there.
  const remote = isRemoteId(video.id);
  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-col gap-3">
        {title}
        <div className="flex flex-wrap items-center gap-2">
          <FavoriteToggle
            variant="page"
            favorite={favorite}
            label={t.player.facts.favorite}
            onToggle={async () => {
              setFailure(null);
              // Confirm the state from refreshed data; the toggle blocks presses until then.
              await onFavorite(!favorite);
            }}
            onFailed={(reason) => setFailure(t.player.facts.favoriteFailed(reason))}
          />
          {!remote && (
            <AddToPlaylistButton
              variant="page"
              label={t.player.facts.playlist}
              videoIds={[video.id]}
            />
          )}
          <AddToGroupButton
            variant="page"
            label={t.player.facts.group}
            videoIds={[video.id]}
          />
          {actions}
        </div>
      </div>
      <FailureLine failure={failure} />
    </div>
  );
}
