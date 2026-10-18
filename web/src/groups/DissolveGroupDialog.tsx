import type { LibraryGroup } from "../api/client";
import { dissolveGroup } from "../api/groups";
import { t } from "../i18n";
import GroupConfirmDialog from "./GroupConfirmDialog";

/**
 * Mount only while open; block closing during submission. Delegate missing groups to onGone and
 * retain other failures in the dialog.
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
