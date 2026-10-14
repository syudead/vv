import type { LibraryGroup } from "../api/client";
import { dissolveGroup } from "../api/groups";
import { t } from "../i18n";
import GroupConfirmDialog from "./GroupConfirmDialog";

/**
 * DissolveGroupDialog はグループを解散する確認の窓である（specs/048-groups/ui-design.md
 * 「Dissolving」）。説明は動画とグループがライブラリに残ることを言い、自動グループ（`folder`
 * あり）にはフォルダが自動でまとまらなくなることを 2 文目に足す（要件 13）。
 *
 * 開いている間だけ描く。確定で `DELETE /api/groups/{id}` を送り、送っている間は閉じない。
 * `204` は `onDissolved`、`404`（もう解散されていた）は `onGone` に任せる。ほかの失敗は窓の中の
 * `Alert` に出し、窓を開いたままにする（GroupConfirmDialog）。
 */
export default function DissolveGroupDialog({
  group,
  onClose,
  onDissolved,
  onGone,
}: {
  group: Pick<LibraryGroup, "id" | "name" | "folder">;
  onClose: () => void;
  onDissolved: () => void;
  onGone: () => void;
}) {
  const words = t.groups.dissolveDialog;
  return (
    <GroupConfirmDialog
      request={() => dissolveGroup(group.id)}
      onDone={onDissolved}
      onClose={onClose}
      onGone={onGone}
      title={words.title(group.name)}
      description={
        group.folder === undefined ? (
          words.description
        ) : (
          <>
            {words.description} {words.folder}
          </>
        )
      }
      submit={words.submit}
      submitting={words.submitting}
      failed={words.failed}
    />
  );
}
