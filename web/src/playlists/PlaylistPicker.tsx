import { ListVideo } from "lucide-react";
import { useEffect, useRef } from "react";

import { RequestFailed } from "../api/client";
import {
  addToPlaylist,
  createPlaylist,
  listAllPlaylists,
  type PlaylistSummary,
} from "../api/playlists";
import { t } from "../i18n";
import NamedPicker, { type PickerChoice } from "../library/NamedPicker";
import { resultText } from "./pickerRules";

const noSmartFolders: readonly number[] = [];

/**
 * createOnce は名前 name のプレイリストを作り、memory に覚える。memory に既にあればそれを返す。
 */
async function createOnce(
  memory: Map<string, PlaylistSummary>,
  name: string,
): Promise<PlaylistSummary> {
  const earlier = memory.get(name);
  if (earlier !== undefined) return earlier;
  const playlist = await createPlaylist(name);
  memory.set(name, playlist);
  return playlist;
}

/**
 * PlaylistPicker は「Add to playlist」のポップオーバーの中身である
 * （specs/044-playlists/ui-design.md「Add to playlist」）。カード・行・動画ページ・選択バーの
 * 4 か所が、それぞれの `Popover` の中に置く。形と読み込みは NamedPicker が持つ。
 *
 * - 開くたびに `GET /api/playlists` を `nextCursor` が尽きるまで読む（listAllPlaylists）。
 * - 行を選ぶと `videoIds` と `groupIds`（と `smartFolderIds`）を `POST /api/playlists/{id}/items` で送る。作成の行は
 *   `POST /api/playlists` の後に、その新しいプレイリストへ足す。足すのに失敗した後に同じ名前で
 *   作成の行をもう一度選ぶと、開いている間に作ったその名前のプレイリストへ足し直す（別の名前を
 *   挟んでも同じ）。そのプレイリストが消えていた（404）なら覚えを捨て、次は作り直す。
 */
export default function PlaylistPicker({
  open,
  onOpenChange,
  videoIds,
  groupIds,
  smartFolderIds = noSmartFolders,
  side = "bottom",
  align = "end",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  videoIds: readonly number[];
  groupIds: readonly number[];
  /** 足すスマートフォルダの id。フォルダのカードと行の「Add to playlist」だけが渡す。 */
  smartFolderIds?: readonly number[];
  side?: "top" | "bottom";
  align?: "start" | "end";
}) {
  // 作成の行で作ったプレイリストを名前ごとに覚える。同じ名前でもう一度押したら、作り直さずに
  // これへ足す。名前は重ねてよいので、作り直すと空のプレイリストが残る。開くたびに新しい Map に
  // 替え、送信は始めたときの Map にだけ書くので、閉じたピッカーの返事が開き直した後の覚えを
  // 書き換えることはない。
  const created = useRef(new Map<string, PlaylistSummary>());
  useEffect(() => {
    if (open) created.current = new Map();
  }, [open]);

  // 送るものは、この描画の props（videoIds・groupIds）で決まる。送っている間に選択が
  // 変わっても、この呼び出しの値は変わらない。
  async function submit(choice: PickerChoice<PlaylistSummary>) {
    const memory = created.current;
    let playlist: { id: number; name: string } | null = null;
    try {
      playlist =
        "target" in choice ? choice.target : await createOnce(memory, choice.create);
      const result = await addToPlaylist(
        playlist.id,
        videoIds,
        groupIds,
        undefined,
        smartFolderIds,
      );
      return resultText(result, playlist.name);
    } catch (error: unknown) {
      // 作ったプレイリストが別のタブで消されていれば、次の作成の行で作り直す。
      if (error instanceof RequestFailed && error.status === 404 && playlist !== null) {
        const gone = playlist.id;
        for (const [name, kept] of memory) {
          if (kept.id === gone) memory.delete(name);
        }
      }
      throw error;
    }
  }

  return (
    <NamedPicker
      open={open}
      onOpenChange={onOpenChange}
      words={t.playlists.add}
      icon={ListVideo}
      memberCount={videoIds.length + groupIds.length + smartFolderIds.length}
      loadAll={listAllPlaylists}
      submit={submit}
      side={side}
      align={align}
    />
  );
}
