import { useRef } from "react";

import { type LibraryGroup, RequestFailed } from "../api/client";
import { createGroup } from "../api/groups";
import { t } from "../i18n";
import NameDialog from "../library/NameDialog";
import { useToast } from "../ui/Toast";
import { useGroupChanged } from "./groupChanges";

/**
 * NewGroupDialog は選択バーの「New group」の窓である。
 * 名前を 1 つ聞き、前後を削った名前と `videoIds`・`groupIds` を `POST /api/groups` で送る。
 * 呼び出し元は開いている間だけ描く。形と名前の規則は NameDialog が持つ。
 *
 * - `201` でトースト「Created "{name}"」を出し、`onCreated` を呼ぶ（呼び出し元が閉じて選択を
 *   外す）。一覧には GroupChangedContext で伝え、一覧が読み直す。
 * - `404 group_target_not_found` は窓の中の `Alert` に出し、閉じたときに一覧を読み直す。
 */
export default function NewGroupDialog({
  videoIds,
  groupIds,
  onClose,
  onCreated,
}: {
  videoIds: readonly number[];
  groupIds: readonly number[];
  onClose: () => void;
  onCreated: (group: LibraryGroup) => void;
}) {
  const toast = useToast();
  const changed = useGroupChanged();
  // 対象が途中で消えていたら、閉じたときに一覧を読み直す。
  const stale = useRef(false);

  return (
    <NameDialog
      words={t.groups.newDialog}
      description={t.groups.newDialog.description(videoIds.length, groupIds.length)}
      nameLabel={t.groups.nameLabel}
      nameRule={t.groups.nameRule}
      send={(name) => createGroup(name, { videoIds, groupIds })}
      onClose={() => {
        if (stale.current) changed(null);
        onClose();
      }}
      onDone={(group) => {
        toast(t.groups.created(group.name));
        changed(null);
        onCreated(group);
      }}
      onFailed={(error) => {
        if (error instanceof RequestFailed && error.status === 404) stale.current = true;
      }}
    />
  );
}
