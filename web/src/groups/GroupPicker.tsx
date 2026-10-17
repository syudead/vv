import { Folder } from "lucide-react";

import { type LibraryGroup } from "../api/client";
import { addToGroup, createGroup, listAllGroups } from "../api/groups";
import { t } from "../i18n";
import NamedPicker, { type PickerChoice } from "../library/NamedPicker";
import { useGroupChanged } from "./groupChanges";

/**
 * GroupPicker は「Add to group」のポップオーバーの中身である。カード・行・動画ページ・選択バーの 4 か所が、
 * それぞれの `Popover` の中に置く。形と読み込みは NamedPicker が持つ。
 *
 * - 開くたびに `GET /api/groups` を `nextCursor` が尽きるまで読み（listAllGroups）、自然な名前の
 *   順に出す。送るグループ自身（`groupIds`）は除く。グループは自分を入れられないからである。
 *   その子孫は出し、サーバーが 409 `group_cycle` で断る（「Cycle」）。
 * - 読み込み中と失敗の間も、打った名前の作成の行を出す。
 * - グループを選ぶと `POST /api/groups/{id}/items`、作成の行は名前とメンバーで `POST /api/groups`。
 *   結果はトースト「Added to "{name}"」か「Created "{name}"」で、引き金の置かれた一覧に
 *   GroupChangedContext で伝える。
 */
export default function GroupPicker({
  open,
  onOpenChange,
  videoIds,
  groupIds,
  side = "bottom",
  align = "end",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  videoIds: readonly number[];
  groupIds: readonly number[];
  side?: "top" | "bottom";
  align?: "start" | "end";
}) {
  const changed = useGroupChanged();

  // 送るものは、この描画の props（videoIds・groupIds）で決まる。
  async function submit(choice: PickerChoice<LibraryGroup>) {
    const members = { videoIds, groupIds };
    if ("target" in choice) {
      await addToGroup(choice.target.id, members);
      changed(choice.target.id);
      return t.groups.add.added(choice.target.name);
    }
    const group = await createGroup(choice.create, members);
    changed(null);
    return t.groups.created(group.name);
  }

  return (
    <NamedPicker
      open={open}
      onOpenChange={onOpenChange}
      words={t.groups.add}
      icon={Folder}
      memberCount={videoIds.length + groupIds.length}
      loadAll={listAllGroups}
      submit={submit}
      excludeIds={groupIds}
      createWhileLoading
      side={side}
      align={align}
    />
  );
}
