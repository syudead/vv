import { type GroupTagResult, tagGroup } from "../api/groups";
import { t } from "../i18n";
import GroupConfirmDialog from "./GroupConfirmDialog";

/**
 * Mount only while open; keep the dialog open while submitting or showing errors, and delegate
 * missing groups to onGone.
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
