import { useRef } from "react";

import { type LibraryGroup, RequestFailed } from "../api/client";
import { createGroup } from "../api/groups";
import { t } from "../i18n";
import NameDialog from "../library/NameDialog";
import { useToast } from "../ui/Toast";
import { useGroupChanged } from "./groupChanges";

/**
 * Mount only while open; on success callers close and clear selection. A missing target keeps the
 * dialog open and triggers a reload when it closes.
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
  // Reload on close when a target disappeared during the operation.
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
