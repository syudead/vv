import { ChevronDown, Heart, HeartOff } from "lucide-react";

import { updateFavorites } from "../api/favorites";
import { maxVideoTagsSelection } from "../api/tags";
import { errorText, t } from "../i18n";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { useToast } from "../ui/Toast";
import { SelectionAction, WithTooltip } from "./SelectionAction";
import { overLimitMessage } from "./selectionErrors";

/** Send selected groups and standalone videos once, preserving selection; member videos are excluded when their group is selected. */
export default function FavoriteMenu({
  videoIds,
  groupIds,
  overLimit,
  overLimitId,
}: {
  videoIds: readonly number[];
  groupIds: readonly number[];
  overLimit: boolean;
  overLimitId: string;
}) {
  const toast = useToast();

  function apply(favorite: boolean) {
    // Recheck current counts before submitting to avoid an over-limit 400.
    if (videoIds.length + groupIds.length > maxVideoTagsSelection) {
      toast(overLimitMessage());
      return;
    }
    void updateFavorites(videoIds, groupIds, favorite).then(
      (result) => {
        const applied = result.appliedVideos + result.appliedGroups;
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
    <DropdownMenu>
      <WithTooltip label={t.library.selection.favorite}>
        <DropdownMenuTrigger asChild>
          <SelectionAction
            icon={<Heart aria-hidden="true" />}
            label={t.library.selection.favorite}
            after={<ChevronDown aria-hidden="true" />}
            disabled={overLimit}
            title={overLimit ? overLimitMessage() : undefined}
            aria-describedby={overLimit ? overLimitId : undefined}
          />
        </DropdownMenuTrigger>
      </WithTooltip>
      <DropdownMenuContent side="top" align="start">
        <DropdownMenuItem onSelect={() => apply(true)}>
          <Heart aria-hidden="true" />
          {t.library.selection.addFavorites}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => apply(false)}>
          <HeartOff aria-hidden="true" />
          {t.library.selection.removeFavorites}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
