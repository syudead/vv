import { Folder } from "lucide-react";

import { type LibraryGroup } from "../api/client";
import { addToGroup, createGroup, listAllGroups } from "../api/groups";
import { t } from "../i18n";
import NamedPicker, { type PickerChoice } from "../library/NamedPicker";
import { useGroupChanged } from "./groupChanges";

/**
 * Reload all candidates on opening and exclude the source groups; the server rejects descendant
 * cycles. Keep creation available during loading and failures.
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

  // Use the current render's videoIds and groupIds for submission.
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
