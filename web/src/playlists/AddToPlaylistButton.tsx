import { ListPlus } from "lucide-react";

import type { UiText } from "../i18n";
import PickerButton from "../library/PickerButton";
import PlaylistPicker from "./PlaylistPicker";

const noIds: readonly number[] = [];

/**
 * AddToPlaylistButton はカード・リスト表示の行・動画ページの「Add to playlist」の引き金と
 * ピッカーである（specs/044-playlists/ui-design.md「Add to playlist」の置き場所の表）。
 * 形は PickerButton が持つ。`card` はハートの左、`row` はお気に入りの列の直後の列、`page` は
 * 動画ページのお気に入りの直後に置く。
 */
export default function AddToPlaylistButton({
  variant,
  label,
  videoIds = noIds,
  groupIds = noIds,
}: {
  variant: "card" | "row" | "page";
  /** 読み上げ名。`page` ではツールチップの文言でもある。 */
  label: UiText;
  videoIds?: readonly number[];
  groupIds?: readonly number[];
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
        />
      )}
    />
  );
}
