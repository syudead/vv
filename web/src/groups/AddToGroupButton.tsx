import { FolderPlus } from "lucide-react";

import type { UiText } from "../i18n";
import PickerButton from "../library/PickerButton";
import GroupPicker from "./GroupPicker";

const noIds: readonly number[] = [];

export default function AddToGroupButton({
  variant,
  label,
  videoIds = noIds,
  groupIds = noIds,
}: {
  variant: "card" | "row" | "page";
  label: UiText;
  videoIds?: readonly number[];
  groupIds?: readonly number[];
}) {
  return (
    <PickerButton
      variant={variant}
      label={label}
      icon={FolderPlus}
      picker={(open, onOpenChange) => (
        <GroupPicker
          open={open}
          onOpenChange={onOpenChange}
          videoIds={videoIds}
          groupIds={groupIds}
        />
      )}
    />
  );
}
