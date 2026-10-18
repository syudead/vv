import { Group, Layers } from "lucide-react";
import { useCallback, useId } from "react";

import { maxBundleSelection } from "../api/client";
import { t } from "../i18n";
import { DropdownMenuItem } from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import type { CardDrop } from "./cardDrag";
import { type DropChoice, DropMenu, useDropMenu } from "./dropMenu";
import { bundleOverLimit } from "./videoDrop";

/** Disable bundling above 20 cards including the target; dismissal without a choice sends no action. */
export function VideoDropMenu({
  name,
  liftedCount,
  onGroup,
  onBundle,
  onClose,
}: {
  name: string;
  liftedCount: number;
  onGroup: () => void;
  onBundle: () => void;
  /** Called on every close, including after selection. */
  onClose: () => void;
}) {
  const overLimit = bundleOverLimit(liftedCount);
  const overLimitId = useId();
  const bundle = (
    <DropdownMenuItem
      disabled={overLimit}
      aria-describedby={overLimit ? overLimitId : undefined}
      onSelect={onBundle}
    >
      <Layers aria-hidden="true" />
      {t.library.selection.bundle}
    </DropdownMenuItem>
  );
  return (
    <DropMenu name={name} onClose={onClose}>
      <DropdownMenuItem onSelect={onGroup}>
        <Group aria-hidden="true" />
        {t.organize.menu.makeGroup}
      </DropdownMenuItem>
      {overLimit ? (
        // Disabled items receive no pointer events, so attach the tooltip to a wrapper.
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="block">{bundle}</span>
          </TooltipTrigger>
          <TooltipContent>
            {t.library.selection.bundleOverLimit(maxBundleSelection)}
          </TooltipContent>
        </Tooltip>
      ) : (
        bundle
      )}
      {overLimit && (
        <span id={overLimitId} className="sr-only">
          {t.library.selection.bundleOverLimit(maxBundleSelection)}
        </span>
      )}
    </DropMenu>
  );
}

export type VideoDropChoice = DropChoice;

/**
 * Retain lifted and target marks while the menu is open; only video targets are accepted, and
 * dismissal performs no action.
 */
export function useVideoDropMenu({
  isVideo,
  nameOf,
  onGroup,
  onBundle,
}: {
  isVideo: (key: string) => boolean;
  nameOf: (key: string) => string;
  onGroup: (choice: VideoDropChoice) => void;
  onBundle: (choice: VideoDropChoice) => void;
}) {
  const accepts = useCallback(({ sign }: CardDrop) => isVideo(sign.key), [isVideo]);
  const menu = useCallback(
    (choice: VideoDropChoice, close: () => void) => (
      <VideoDropMenu
        name={nameOf(choice.target)}
        liftedCount={choice.lifted.length}
        onGroup={() => onGroup(choice)}
        onBundle={() => onBundle(choice)}
        onClose={close}
      />
    ),
    [nameOf, onBundle, onGroup],
  );
  return useDropMenu(accepts, menu);
}
