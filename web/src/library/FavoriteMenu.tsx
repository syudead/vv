import { ChevronDown, Heart, HeartOff } from "lucide-react";

import type { FolderRef } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { maxVideoTagsSelection } from "../api/tags";
import { errorText, t } from "../i18n";
import Button from "../ui/Button";
import { MenuContent, MenuItem, MenuRoot, MenuTrigger } from "../ui/Menu";
import { useToast } from "../ui/Toast";
import { overLimitMessage } from "./selectionErrors";

/**
 * FavoriteMenu は選択バーの「Favorite」である（specs/035-favorites/ui-design.md
 * 「Selection bar」）。選んだ項目の今の状態は示さず、「Add to favorites」「Remove from
 * favorites」の両方を常に押せる。`videoIds` は選んだ id のうち選んだグループのメンバーで
 * ないもの、`folders` は選んだグループで、1 回の `PUT /api/favorites` で送る（research.md
 * R-7）。確定したらトーストで結果を伝え、選択は残す。カードの印は応答の通知
 * （api/favorites.ts）で一覧が差し替える。
 */
export default function FavoriteMenu({
  videoIds,
  folders,
  overLimit,
  overLimitId,
  className,
  ...rest
}: {
  videoIds: readonly number[];
  folders: readonly FolderRef[];
  /** 送る数（`videoIds` と `folders` の合計）が上限を超えるとき true。 */
  overLimit: boolean;
  overLimitId: string;
  className?: string;
  /** 選択バーが 1 行に収まるかを測る目印（SelectionBar の measureBarLayout）。 */
  "data-bar-item"?: string;
}) {
  const toast = useToast();

  function apply(favorite: boolean) {
    // 送る直前に最新の数を確かめる（タグ・公開と同じ理由。上限を超えると 400 になる）。
    if (videoIds.length + folders.length > maxVideoTagsSelection) {
      toast(overLimitMessage());
      return;
    }
    void updateFavorites(videoIds, folders, favorite).then(
      (result) => {
        const applied = result.appliedVideos + result.appliedFolders;
        toast(
          favorite
            ? t.library.selection.favoritesAdded(applied)
            : t.library.selection.favoritesRemoved(applied),
        );
      },
      (error: unknown) => toast(t.library.selection.favoritesFailed(errorText(error))),
    );
  }

  return (
    <MenuRoot>
      <MenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className={className}
          disabled={overLimit}
          title={overLimit ? overLimitMessage() : undefined}
          aria-describedby={overLimit ? overLimitId : undefined}
          {...rest}
        >
          <Heart aria-hidden="true" />
          {t.library.selection.favorite}
          <ChevronDown aria-hidden="true" />
        </Button>
      </MenuTrigger>
      <MenuContent side="top" align="start">
        <MenuItem onSelect={() => apply(true)}>
          <Heart aria-hidden="true" />
          {t.library.selection.addFavorites}
        </MenuItem>
        <MenuItem onSelect={() => apply(false)}>
          <HeartOff aria-hidden="true" />
          {t.library.selection.removeFavorites}
        </MenuItem>
      </MenuContent>
    </MenuRoot>
  );
}
