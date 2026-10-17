import { FolderPlus } from "lucide-react";

import { t, type UiText } from "../i18n";
import PickerButton from "../library/PickerButton";
import GroupPicker from "./GroupPicker";

const noIds: readonly number[] = [];

/**
 * AddToGroupButton はカード・リスト表示の行・動画ページの「Add to group」の引き金とピッカーで
 * ある。形は PickerButton
 * が持つ。`card` は「Add to playlist」の左、`row` は「Add to playlist」の列の直後の列、`page` は
 * 動画ページの「Add to playlist」の直後に置き、ツールチップは「Add to group」。
 */
export default function AddToGroupButton({
  variant,
  label,
  videoIds = noIds,
  groupIds = noIds,
}: {
  variant: "card" | "row" | "page";
  /** 読み上げ名（「Add "{title}" to a group」など）。 */
  label: UiText;
  videoIds?: readonly number[];
  groupIds?: readonly number[];
}) {
  return (
    <PickerButton
      variant={variant}
      label={label}
      tooltip={t.groups.add.label}
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
