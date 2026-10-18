import {
  FolderMinus,
  FolderPlus,
  Group,
  Layers,
  ListPlus,
  Minus,
  Plus,
} from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";

import { maxBundleSelection, type VideoVersions } from "../api/client";
import { maxVideoTagsSelection } from "../api/tags";
import GroupPicker from "../groups/GroupPicker";
import NewGroupDialog from "../groups/NewGroupDialog";
import { t, type UiText } from "../i18n";
import PlaylistPicker from "../playlists/PlaylistPicker";
import { SelectionBar as SelectionBarSection } from "../ui/patterns/selection-bar";
import { Popover, PopoverTrigger } from "../ui/shadcn/popover";
import BundleDialog from "../versions/BundleDialog";
import AddTagPopover from "./AddTagPopover";
import FavoriteMenu from "./FavoriteMenu";
import RemoveTagPopover from "./RemoveTagPopover";
import { SelectionAction, WithTooltip } from "./SelectionAction";
import { overLimitMessage } from "./selectionErrors";
import VisibilityMenu from "./VisibilityMenu";

function PickerAction({
  icon,
  label,
  overLimit,
  overLimitId,
  open,
  onOpenChange,
  children,
}: {
  icon: ReactNode;
  label: UiText;
  overLimit: boolean;
  overLimitId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
}) {
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <WithTooltip label={label}>
        <PopoverTrigger asChild>
          <SelectionAction
            icon={icon}
            label={label}
            disabled={overLimit}
            title={overLimit ? overLimitMessage() : undefined}
            aria-describedby={overLimit ? overLimitId : undefined}
          />
        </PopoverTrigger>
      </WithTooltip>
      {children}
    </Popover>
  );
}

export interface SelectionBarProps {
  count: number;
  selectedIds: readonly number[];
  /** Send standalone video IDs plus selected group IDs; limits apply to that combined request count. */
  favoriteVideoIds: readonly number[];
  favoriteGroupIds: readonly number[];
  onClear: () => void;
  /** Removing an active filter tag lets callers clear selection and reload matching rows. */
  onTagRemoved: (tagId: number) => void;
  onBundled: (versions: VideoVersions) => void;
  /** Group-only removal; disable oversized batches instead of splitting them. */
  removeFromGroup?: {
    count: number;
    pending: boolean;
    onRemove: () => void;
  };
}

export default function SelectionBar({
  count,
  selectedIds,
  favoriteVideoIds,
  favoriteGroupIds,
  onClear,
  onTagRemoved,
  onBundled,
  removeFromGroup,
}: SelectionBarProps) {
  const [addOpen, setAddOpen] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);
  const [playlistOpen, setPlaylistOpen] = useState(false);
  const [groupOpen, setGroupOpen] = useState(false);
  const [newGroupOpen, setNewGroupOpen] = useState(false);
  const addTriggerRef = useRef<HTMLButtonElement | null>(null);

  // Batch APIs are atomic; disable oversized selections with a reason instead of silently splitting.
  const overLimitId = useId();
  const overLimit = count > maxVideoTagsSelection;
  // Favorite limits count groups as one target, unlike tag/visibility operations on all members.
  //
  const favoriteOverLimitId = useId();
  const favoriteOverLimit =
    favoriteVideoIds.length + favoriteGroupIds.length > maxVideoTagsSelection;

  // Freeze dialog IDs at opening; inert background content prevents selection changes.
  const [bundleOpen, setBundleOpen] = useState(false);
  const bundleOverLimitId = useId();
  const canBundle = count >= 2;
  const bundleOverLimit = count > maxBundleSelection;
  const [bundleIds, setBundleIds] = useState<readonly number[]>([]);

  // SelectionBar survives returning null; close popovers at zero so reselecting cannot reopen them unexpectedly.
  useEffect(() => {
    if (count === 0) {
      setAddOpen(false);
      setRemoveOpen(false);
      setPlaylistOpen(false);
      setGroupOpen(false);
      setNewGroupOpen(false);
      setBundleOpen(false);
    }
  }, [count]);

  // Recheck picker limits as selection grows and close over-limit popovers.
  useEffect(() => {
    if (favoriteOverLimit) {
      setPlaylistOpen(false);
      setGroupOpen(false);
    }
  }, [favoriteOverLimit]);

  // A group alone cannot form a new group: that would only add another hierarchy level.
  const newGroupNeedsMoreId = useId();
  const newGroupNeedsMore =
    favoriteGroupIds.length === 1 && favoriteVideoIds.length === 0;
  const newGroupReason = favoriteOverLimit
    ? overLimitMessage()
    : newGroupNeedsMore
      ? t.library.selection.newGroupNeedsMore
      : undefined;

  // Disabling a trigger does not close its popover when selection exceeds the limit.
  // Guard the render before the closing effect runs.
  useEffect(() => {
    if (overLimit) {
      setAddOpen(false);
      setRemoveOpen(false);
    }
  }, [overLimit]);

  if (count === 0) return null;

  return (
    // Keep the region wrapper from changing the bar's bottom alignment.
    <div role="region" aria-label={t.library.selection.region} className="contents">
      <SelectionBarSection
        count={t.library.selection.count(count)}
        clearLabel={t.library.selection.clear}
        onClear={onClear}
      >
        <Popover open={addOpen} onOpenChange={setAddOpen}>
          <WithTooltip label={t.library.selection.addTag}>
            <PopoverTrigger asChild>
              <SelectionAction
                ref={addTriggerRef}
                icon={<Plus aria-hidden="true" />}
                label={t.library.selection.addTag}
                disabled={overLimit}
                title={overLimit ? overLimitMessage() : undefined}
                aria-describedby={overLimit ? overLimitId : undefined}
              />
            </PopoverTrigger>
          </WithTooltip>
          <AddTagPopover
            open={addOpen}
            onOpenChange={setAddOpen}
            selectedIds={selectedIds}
            onDone={() => addTriggerRef.current?.focus()}
          />
        </Popover>

        <Popover open={removeOpen} onOpenChange={setRemoveOpen}>
          <WithTooltip label={t.library.selection.removeTag}>
            <PopoverTrigger asChild>
              <SelectionAction
                icon={<Minus aria-hidden="true" />}
                label={t.library.selection.removeTag}
                disabled={overLimit}
                title={overLimit ? overLimitMessage() : undefined}
                aria-describedby={overLimit ? overLimitId : undefined}
              />
            </PopoverTrigger>
          </WithTooltip>
          <RemoveTagPopover
            open={removeOpen}
            onOpenChange={setRemoveOpen}
            selectedIds={selectedIds}
            onRemoved={onTagRemoved}
          />
        </Popover>
        {overLimit && (
          <span id={overLimitId} className="sr-only">
            {overLimitMessage()}
          </span>
        )}

        <FavoriteMenu
          videoIds={favoriteVideoIds}
          groupIds={favoriteGroupIds}
          overLimit={favoriteOverLimit}
          overLimitId={favoriteOverLimitId}
        />
        {favoriteOverLimit && (
          <span id={favoriteOverLimitId} className="sr-only">
            {overLimitMessage()}
          </span>
        )}

        {}
        <PickerAction
          icon={<ListPlus aria-hidden="true" />}
          label={t.library.selection.addToPlaylist}
          overLimit={favoriteOverLimit}
          overLimitId={favoriteOverLimitId}
          open={playlistOpen}
          onOpenChange={setPlaylistOpen}
        >
          <PlaylistPicker
            open={playlistOpen}
            onOpenChange={setPlaylistOpen}
            videoIds={favoriteVideoIds}
            groupIds={favoriteGroupIds}
            side="top"
            align="start"
          />
        </PickerAction>

        {}
        <WithTooltip label={newGroupReason ?? t.library.selection.newGroup}>
          <SelectionAction
            icon={<Group aria-hidden="true" />}
            label={t.library.selection.newGroup}
            disabled={newGroupReason !== undefined}
            title={newGroupReason}
            aria-describedby={
              favoriteOverLimit
                ? favoriteOverLimitId
                : newGroupNeedsMore
                  ? newGroupNeedsMoreId
                  : undefined
            }
            onClick={() => setNewGroupOpen(true)}
          />
        </WithTooltip>
        {newGroupNeedsMore && (
          <span id={newGroupNeedsMoreId} className="sr-only">
            {t.library.selection.newGroupNeedsMore}
          </span>
        )}
        {newGroupOpen && (
          <NewGroupDialog
            videoIds={favoriteVideoIds}
            groupIds={favoriteGroupIds}
            onClose={() => setNewGroupOpen(false)}
            onCreated={() => {
              setNewGroupOpen(false);
              onClear();
            }}
          />
        )}

        <PickerAction
          icon={<FolderPlus aria-hidden="true" />}
          label={t.library.selection.addToGroup}
          overLimit={favoriteOverLimit}
          overLimitId={favoriteOverLimitId}
          open={groupOpen}
          onOpenChange={setGroupOpen}
        >
          <GroupPicker
            open={groupOpen}
            onOpenChange={setGroupOpen}
            videoIds={favoriteVideoIds}
            groupIds={favoriteGroupIds}
            side="top"
            align="start"
          />
        </PickerAction>

        {}
        {removeFromGroup !== undefined && <RemoveFromGroupAction {...removeFromGroup} />}

        <VisibilityMenu
          selectedIds={selectedIds}
          overLimit={overLimit}
          overLimitId={overLimitId}
        />

        {canBundle && (
          <WithTooltip label={t.library.selection.bundle}>
            <SelectionAction
              icon={<Layers aria-hidden="true" />}
              label={t.library.selection.bundle}
              disabled={bundleOverLimit}
              title={
                bundleOverLimit
                  ? t.library.selection.bundleOverLimit(maxBundleSelection)
                  : undefined
              }
              aria-describedby={bundleOverLimit ? bundleOverLimitId : undefined}
              onClick={() => {
                setBundleIds(selectedIds);
                setBundleOpen(true);
              }}
            />
          </WithTooltip>
        )}
        {canBundle && bundleOverLimit && (
          <span id={bundleOverLimitId} className="sr-only">
            {t.library.selection.bundleOverLimit(maxBundleSelection)}
          </span>
        )}
        {bundleOpen && (
          <BundleDialog
            videoIds={bundleIds}
            onClose={() => setBundleOpen(false)}
            onBundled={(versions) => {
              setBundleOpen(false);
              onBundled(versions);
            }}
          />
        )}
      </SelectionBarSection>
    </div>
  );
}

function RemoveFromGroupAction({
  count,
  pending,
  onRemove,
}: NonNullable<SelectionBarProps["removeFromGroup"]>) {
  const overLimitId = useId();
  const overLimit = count > maxVideoTagsSelection;
  return (
    <>
      <WithTooltip label={t.library.selection.removeFromGroup}>
        <SelectionAction
          icon={<FolderMinus aria-hidden="true" />}
          label={t.library.selection.removeFromGroup}
          disabled={count === 0 || pending || overLimit}
          title={overLimit ? overLimitMessage() : undefined}
          aria-describedby={overLimit ? overLimitId : undefined}
          onClick={onRemove}
        />
      </WithTooltip>
      {overLimit && (
        <span id={overLimitId} className="sr-only">
          {overLimitMessage()}
        </span>
      )}
    </>
  );
}
