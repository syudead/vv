import { ListPlus } from "lucide-react";

import type { UiText } from "../i18n";
import PickerButton from "../library/PickerButton";
import PlaylistPicker from "./PlaylistPicker";

const noIds: readonly number[] = [];

export default function AddToPlaylistButton({
  variant,
  label,
  videoIds = noIds,
  groupIds = noIds,
  smartFolderIds = noIds,
}: {
  variant: "card" | "row" | "page";
  /** Also used as the tooltip for page placement. */
  label: UiText;
  videoIds?: readonly number[];
  groupIds?: readonly number[];
  smartFolderIds?: readonly number[];
}) {
  return (
    <PickerButton
      variant={variant}
      label={label}
      icon={ListPlus}
      picker={(open, onOpenChange) => (
        <PlaylistPicker
          open={open}
          onOpenChange={onOpenChange}
          videoIds={videoIds}
          groupIds={groupIds}
          smartFolderIds={smartFolderIds}
        />
      )}
    />
  );
}
