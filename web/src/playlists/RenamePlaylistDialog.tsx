import { renamePlaylist, type PlaylistSummary } from "../api/playlists";
import { t } from "../i18n";
import RenameDialog from "../videoList/RenameDialog";
import { playlistName } from "./playlistName";

/** Keep open during submission; callers handle closing after rename or a missing target. */
export default function RenamePlaylistDialog({
  playlistId,
  currentName,
  onClose,
  onRenamed,
  onGone,
}: {
  playlistId: number;
  currentName: string;
  onClose: () => void;
  onRenamed: (playlist: PlaylistSummary) => void;
  onGone: () => void;
}) {
  return (
    <RenameDialog
      currentName={currentName}
      texts={{
        ...t.playlists.renameDialog,
        nameLabel: t.playlists.nameLabel,
        nameRule: t.playlists.nameRule,
      }}
      normalize={playlistName}
      rename={(name) => renamePlaylist(playlistId, name)}
      onClose={onClose}
      onRenamed={onRenamed}
      onGone={onGone}
    />
  );
}
