import { type ReactNode, useState } from "react";

import type { Video } from "../api/client";
import AddToGroupButton from "../groups/AddToGroupButton";
import { t, type UiText } from "../i18n";
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
}: {
  video: Video;
  title: ReactNode;
  /** Resolve after toggling and refreshing the video; reject on failure. Absent for guests. */
  onFavorite: ((favorite: boolean) => Promise<void>) | undefined;
}) {
  const [failure, setFailure] = useState<UiText | null>(null);
  if (onFavorite === undefined) return title;
  const favorite = video.favorite === true;
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
          <AddToPlaylistButton
            variant="page"
            label={t.playlists.add.label}
            videoIds={[video.id]}
          />
          <AddToGroupButton
            variant="page"
            label={t.groups.add.label}
            videoIds={[video.id]}
          />
        </div>
      </div>
      <FailureLine failure={failure} />
    </div>
  );
}
