import { ChevronDown, Globe, Lock } from "lucide-react";

import { maxVideoTagsSelection } from "../api/tags";
import { updateVideoVisibility } from "../api/visibility";
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

export default function VisibilityMenu({
  selectedIds,
  overLimit,
  overLimitId,
}: {
  selectedIds: readonly number[];
  overLimit: boolean;
  overLimitId: string;
}) {
  const toast = useToast();

  function apply(isPublic: boolean) {
    // Recheck the latest selection count: an oversized atomic request fails entirely.
    if (selectedIds.length > maxVideoTagsSelection) {
      toast(overLimitMessage());
      return;
    }
    void updateVideoVisibility(selectedIds, isPublic).then(
      (result) => {
        toast(
          isPublic
            ? t.library.selection.madePublic(result.applied)
            : t.library.selection.madePrivate(result.applied),
        );
      },
      (error: unknown) => toast(t.library.selection.visibilityFailed(errorText(error))),
    );
  }

  return (
    <DropdownMenu>
      <WithTooltip label={t.library.selection.visibility}>
        <DropdownMenuTrigger asChild>
          <SelectionAction
            icon={<Globe aria-hidden="true" />}
            label={t.library.selection.visibility}
            after={<ChevronDown aria-hidden="true" />}
            disabled={overLimit}
            title={overLimit ? overLimitMessage() : undefined}
            aria-describedby={overLimit ? overLimitId : undefined}
          />
        </DropdownMenuTrigger>
      </WithTooltip>
      <DropdownMenuContent side="top" align="start">
        <DropdownMenuItem onSelect={() => apply(true)}>
          <Globe aria-hidden="true" />
          {t.library.selection.makePublic}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => apply(false)}>
          <Lock aria-hidden="true" />
          {t.library.selection.makePrivate}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
