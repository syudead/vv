import { FolderMinus, FolderPlus, Group, Layers, ListPlus, Pencil } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";

import { maxBundleSelection, type VideoVersions } from "../api/client";
import { maxVideoTagsSelection } from "../api/tags";
import GroupPicker from "../groups/GroupPicker";
import NewGroupDialog from "../groups/NewGroupDialog";
import { t, type UiText } from "../i18n";
import PlaylistPicker from "../playlists/PlaylistPicker";
import { SelectionBar as SelectionBarSection } from "../ui/patterns/selection-bar";
import { Popover, PopoverTrigger } from "../ui/shadcn/popover";
import BundleDialog from "../versions/BundleDialog";
import BulkEditor from "./bulkEdit/BulkEditor";
import DeleteCollectionsAction from "./DeleteCollectionsAction";
import FavoriteMenu from "./FavoriteMenu";
import {
  DisabledReason,
  disabledProps,
  SelectionAction,
  WithTooltip,
} from "./SelectionAction";
import type { SelectionTarget } from "./selection";
import { overLimitMessage } from "./selectionErrors";

function PickerAction({
  icon,
  label,
  disabledReason,
  reasonId,
  open,
  onOpenChange,
  children,
}: {
  icon: ReactNode;
  label: UiText;
  disabledReason: UiText | undefined;
  reasonId: string;
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
            {...disabledProps(disabledReason, reasonId)}
          />
        </PopoverTrigger>
      </WithTooltip>
      {children}
    </Popover>
  );
}

export interface SelectionBarProps {
  /** Selected videos, counting each member of a selected group. */
  count: number;
  /** The selected cards in display order, for the bulk editor's stage. */
  targets: readonly SelectionTarget[];
  selectedIds: readonly number[];
  /** Send standalone video IDs plus selected group IDs; limits apply to that combined request count. */
  favoriteVideoIds: readonly number[];
  favoriteGroupIds: readonly number[];
  /**
   * Favorites and playlists take the folders themselves; tag and visibility actions reach their
   * current members on the server. Group and bundle actions are disabled while any is selected.
   */
  smartFolderIds?: readonly number[];
  onClear: () => void;
  /** A bulk edit removed this tag; an active filter on it lets callers clear selection and reload matching rows. */
  onTagRemoved: (tagId: number) => void;
  onBundled: (versions: VideoVersions) => void;
  /** After selected groups were dissolved and smart folders deleted; callers clear the selection and reload. */
  onCollectionsDeleted: () => void;
  /** Group-only removal; disable oversized batches instead of splitting them. */
  removeFromGroup?: {
    count: number;
    pending: boolean;
    onRemove: () => void;
  };
}

const noSmartFolders: readonly number[] = [];

/**
 * Batch APIs are atomic, so oversized selections disable an action with a reason instead of
 * being split. Favorite limits count a group or smart folder as one target, unlike tag and
 * visibility operations on every member video. Smart-folder members are unknown here, so the
 * server enforces the tag and visibility limit on their resolved total.
 */
export function selectionReasons({
  count,
  favoriteVideoIds,
  favoriteGroupIds,
  smartFolderIds,
}: {
  count: number;
  favoriteVideoIds: readonly number[];
  favoriteGroupIds: readonly number[];
  smartFolderIds: readonly number[];
}) {
  // A smart folder cannot be placed in a group or bundled as a version.
  const videoOnly =
    smartFolderIds.length > 0 ? t.library.selection.smartFoldersUnsupported : undefined;
  const favoriteOverLimit =
    favoriteVideoIds.length + favoriteGroupIds.length + smartFolderIds.length >
    maxVideoTagsSelection;
  const favorite = favoriteOverLimit ? overLimitMessage() : undefined;
  const group = favorite ?? videoOnly;
  // A group alone cannot form a new group: that would only add another hierarchy level.
  const newGroupNeedsMore =
    favoriteGroupIds.length === 1 && favoriteVideoIds.length === 0;
  const collectionCount = favoriteGroupIds.length + smartFolderIds.length;
  return {
    tag: count > maxVideoTagsSelection ? overLimitMessage() : undefined,
    favorite,
    group,
    newGroup:
      group ?? (newGroupNeedsMore ? t.library.selection.newGroupNeedsMore : undefined),
    bundle:
      videoOnly ??
      (count > maxBundleSelection
        ? t.library.selection.bundleOverLimit(maxBundleSelection)
        : undefined),
    // Only collections are deleted; videos selected alongside them stay.
    deleteCollections:
      collectionCount === 0
        ? t.library.selection.deleteNeedsCollection
        : collectionCount > maxVideoTagsSelection
          ? overLimitMessage()
          : undefined,
  };
}

function countText(count: number, smartFolderCount: number): UiText {
  return smartFolderCount === 0
    ? t.library.selection.count(count)
    : t.library.selection.countWithSmartFolders(count, smartFolderCount);
}

export default function SelectionBar({
  count,
  targets,
  selectedIds,
  favoriteVideoIds,
  favoriteGroupIds,
  smartFolderIds = noSmartFolders,
  onClear,
  onTagRemoved,
  onBundled,
  onCollectionsDeleted,
  removeFromGroup,
}: SelectionBarProps) {
  const [editOpen, setEditOpen] = useState(false);
  const [playlistOpen, setPlaylistOpen] = useState(false);
  const [groupOpen, setGroupOpen] = useState(false);
  const [newGroupOpen, setNewGroupOpen] = useState(false);
  const total = count + smartFolderIds.length;
  const reasons = selectionReasons({
    count,
    favoriteVideoIds,
    favoriteGroupIds,
    smartFolderIds,
  });
  const tagReasonId = useId();
  const favoriteReasonId = useId();
  const groupReasonId = useId();
  const newGroupReasonId = useId();
  const bundleReasonId = useId();

  // Freeze dialog IDs at opening; inert background content prevents selection changes.
  const [bundleOpen, setBundleOpen] = useState(false);
  const canBundle = count >= 2;
  const [bundleIds, setBundleIds] = useState<readonly number[]>([]);

  // SelectionBar survives returning null; close popovers at zero so reselecting cannot reopen them unexpectedly.
  useEffect(() => {
    if (total === 0) {
      setEditOpen(false);
      setPlaylistOpen(false);
      setGroupOpen(false);
      setNewGroupOpen(false);
      setBundleOpen(false);
    }
  }, [total]);

  // Disabling a trigger does not close its popover when the selection changes; close it here.
  const tagBlocked = reasons.tag !== undefined;
  const favoriteBlocked = reasons.favorite !== undefined;
  const groupBlocked = reasons.group !== undefined;
  useEffect(() => {
    if (tagBlocked) setEditOpen(false);
  }, [tagBlocked]);
  useEffect(() => {
    if (favoriteBlocked) setPlaylistOpen(false);
  }, [favoriteBlocked]);
  useEffect(() => {
    if (groupBlocked) setGroupOpen(false);
  }, [groupBlocked]);

  if (total === 0) return null;

  return (
    // Keep the region wrapper from changing the bar's bottom alignment.
    <div role="region" aria-label={t.library.selection.region} className="contents">
      <SelectionBarSection
        count={countText(count, smartFolderIds.length)}
        clearLabel={t.library.selection.clear}
        onClear={onClear}
      >
        <WithTooltip label={reasons.tag ?? t.library.selection.edit}>
          <SelectionAction
            icon={<Pencil aria-hidden="true" />}
            label={t.library.selection.edit}
            {...disabledProps(reasons.tag, tagReasonId)}
            onClick={() => setEditOpen(true)}
          />
        </WithTooltip>
        <DisabledReason id={tagReasonId} reason={reasons.tag} />
        {editOpen && (
          <BulkEditor
            targets={targets}
            onClose={() => setEditOpen(false)}
            onApplied={(removed) => {
              for (const tagId of removed) onTagRemoved(tagId);
            }}
          />
        )}

        <FavoriteMenu
          videoIds={favoriteVideoIds}
          groupIds={favoriteGroupIds}
          smartFolderIds={smartFolderIds}
          overLimit={favoriteBlocked}
          overLimitId={favoriteReasonId}
        />
        <DisabledReason id={favoriteReasonId} reason={reasons.favorite} />

        <PickerAction
          icon={<ListPlus aria-hidden="true" />}
          label={t.library.selection.addToPlaylist}
          disabledReason={reasons.favorite}
          reasonId={favoriteReasonId}
          open={playlistOpen}
          onOpenChange={setPlaylistOpen}
        >
          <PlaylistPicker
            open={playlistOpen}
            onOpenChange={setPlaylistOpen}
            videoIds={favoriteVideoIds}
            groupIds={favoriteGroupIds}
            smartFolderIds={smartFolderIds}
            side="top"
            align="start"
          />
        </PickerAction>

        <WithTooltip label={reasons.newGroup ?? t.library.selection.newGroup}>
          <SelectionAction
            icon={<Group aria-hidden="true" />}
            label={t.library.selection.newGroup}
            {...disabledProps(reasons.newGroup, newGroupReasonId)}
            onClick={() => setNewGroupOpen(true)}
          />
        </WithTooltip>
        <DisabledReason id={newGroupReasonId} reason={reasons.newGroup} />
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
          disabledReason={reasons.group}
          reasonId={groupReasonId}
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
        <DisabledReason id={groupReasonId} reason={reasons.group} />

        {removeFromGroup !== undefined && <RemoveFromGroupAction {...removeFromGroup} />}

        {canBundle && (
          <WithTooltip label={t.library.selection.bundle}>
            <SelectionAction
              icon={<Layers aria-hidden="true" />}
              label={t.library.selection.bundle}
              {...disabledProps(reasons.bundle, bundleReasonId)}
              onClick={() => {
                setBundleIds(selectedIds);
                setBundleOpen(true);
              }}
            />
          </WithTooltip>
        )}
        {canBundle && <DisabledReason id={bundleReasonId} reason={reasons.bundle} />}
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

        <DeleteCollectionsAction
          groupIds={favoriteGroupIds}
          smartFolderIds={smartFolderIds}
          disabledReason={reasons.deleteCollections}
          onDeleted={onCollectionsDeleted}
        />
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
