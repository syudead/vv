import { createPlaylist, type PlaylistSummary } from "../api/playlists";
import { t } from "../i18n";
import NameDialog from "../library/NameDialog";

/** Mount only while open to discard names on close; callers own closing and insertion after onCreated. */
export default function NewPlaylistDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (playlist: PlaylistSummary) => void;
}) {
  return (
    <NameDialog
      words={t.playlists.newDialog}
      nameLabel={t.playlists.nameLabel}
      nameRule={t.playlists.nameRule}
      send={(name) => createPlaylist(name)}
      onClose={onClose}
      onDone={onCreated}
    />
  );
}
