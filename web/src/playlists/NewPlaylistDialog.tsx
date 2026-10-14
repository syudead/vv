import { createPlaylist, type PlaylistSummary } from "../api/playlists";
import { t } from "../i18n";
import NameDialog from "../library/NameDialog";

/**
 * NewPlaylistDialog は名前を 1 つ聞いて空のプレイリストを作る窓である
 * （specs/044-playlists/ui-design.md「New playlist」）。呼び出し元は開いている間だけ描き、
 * 閉じるたびに入れた名前を捨てる。形と名前の規則は NameDialog が持つ。
 *
 * `201` で `onCreated` を呼び、閉じるのと新しい行は呼び出し元に任せる。
 */
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
