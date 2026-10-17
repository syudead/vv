import { type GroupTagResult, tagGroup } from "../api/groups";
import { t } from "../i18n";
import GroupConfirmDialog from "./GroupConfirmDialog";

/**
 * GroupTagDialog はグループをタグに変える確認の窓である。グループページの「More」、動画ページのグループ操作、
 * フォルダのまとめ方のメニューの 3 つの入り口が、同じ文のこの窓を開く。
 *
 * 開いている間だけ描く。確定で `POST /api/groups/{id}/tag` を送り、送っている間は閉じない。
 * `200` は `onTagged` に結果を渡し（閉じるのと、トーストと、画面の動きは呼び出し側が持つ）、
 * `404`（途中で解散された）は `onGone` に任せる。ほかの失敗（`400 invalid_request` はタグ名の規則が
 * 理由）は窓の中の `Alert` に出し、窓を開いたままにする（GroupConfirmDialog）。
 */
export default function GroupTagDialog({
  group,
  onClose,
  onTagged,
  onGone,
}: {
  group: { id: number; name: string };
  onClose: () => void;
  onTagged: (result: GroupTagResult) => void;
  onGone: () => void;
}) {
  const words = t.groups.tagDialog;
  return (
    <GroupConfirmDialog
      request={() => tagGroup(group.id)}
      onDone={onTagged}
      onClose={onClose}
      onGone={onGone}
      title={words.title(group.name)}
      description={words.description(group.name)}
      submit={words.submit}
      submitting={words.submitting}
      failed={words.failed}
    />
  );
}
