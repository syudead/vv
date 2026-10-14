import { renamePlaylist, type PlaylistSummary } from "../api/playlists";
import { t } from "../i18n";
import RenameDialog from "../videoList/RenameDialog";
import { playlistName } from "./playlistName";

/**
 * RenamePlaylistDialog はプレイリストの名前を変える窓である
 * （specs/044-playlists/ui-design.md「Renaming」）。名前の欄と規則は新規作成と同じで、
 * 今の名前を入れて選んだ状態で開く。トリムした名前が今の名前と同じ間は「Save」を押せない。
 *
 * `200` で `onRenamed`、`404` で `onGone` を呼ぶ。閉じるのは呼び出し元に任せる。送信中は閉じない。
 */
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
