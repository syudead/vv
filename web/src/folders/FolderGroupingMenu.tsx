import { ChevronDown, Group, LoaderCircle, Tag, Ungroup } from "lucide-react";
import { useState } from "react";

import { type FolderRef, getGroup, RequestFailed } from "../api/client";
import {
  type FolderGrouping,
  type FolderGroupingMode,
  groupingFailure,
  setFolderGrouping,
} from "../api/folderGrouping";
import { taggedMessage } from "../api/groups";
import { clearListSnapshot } from "../api/listSnapshot";
import GroupTagDialog from "../groups/GroupTagDialog";
import { errorText, t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { useToast } from "../ui/Toast";

const modes: readonly FolderGroupingMode[] = ["auto", "ungroup", "groupDirect"];

/**
 * Fetch the group's current name before opening the tag dialog because groups can be renamed. A
 * missing group or grouping conflict calls onStale; tagging calls onTagged to refresh videos.
 */
export default function FolderGroupingMenu({
  folder,
  name,
  grouping,
  onGroupingChange,
  onTagged,
  onStale,
}: {
  folder: FolderRef;
  name: string;
  grouping: FolderGrouping;
  onGroupingChange: (grouping: FolderGrouping) => void;
  onTagged: () => void;
  onStale: () => void;
}) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [tagging, setTagging] = useState<{ id: number; name: string } | null>(null);

  const fail = (failure: unknown) => {
    const { message, conflict } = groupingFailure(failure, {
      name,
      tagging: false,
      notFoundMessage: t.folders.notFound.title,
    });
    toast(message);
    if (conflict) onStale();
  };

  // Send even when the selected value is unchanged.
  const choose = async (mode: FolderGroupingMode) => {
    setBusy(true);
    try {
      onGroupingChange(await setFolderGrouping(folder, mode));
    } catch (failure) {
      fail(failure);
    } finally {
      setBusy(false);
    }
  };

  // The group was dissolved; discard the cached library card.
  const gone = () => {
    setTagging(null);
    clearListSnapshot();
    onStale();
  };

  const openTagDialog = async (id: number) => {
    try {
      const detail = await getGroup(id);
      setTagging({ id, name: detail.group.name });
    } catch (failure) {
      if (failure instanceof RequestFailed && failure.status === 404) gone();
      else toast(t.groups.tagDialog.failed(errorText(failure)));
    }
  };

  const groupId = grouping.groupId;
  const grouped = groupId !== undefined;
  const Icon = busy ? LoaderCircle : grouped ? Group : Ungroup;
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            aria-busy={busy}
            aria-label={
              busy ? t.folders.grouping.changing : t.folders.grouping.trigger(grouped)
            }
          >
            <Icon
              aria-hidden="true"
              className={busy ? "animate-spin motion-reduce:animate-none" : undefined}
            />
            {busy
              ? t.folders.grouping.changingShort
              : grouped
                ? t.folders.grouping.grouped
                : t.folders.grouping.ungrouped}
            <ChevronDown aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="max-w-popover-wide">
          <DropdownMenuLabel>{t.folders.grouping.heading}</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={grouping.mode}
            onValueChange={(mode) => void choose(mode as FolderGroupingMode)}
          >
            {modes.map((mode) => (
              <DropdownMenuRadioItem key={mode} value={mode}>
                {t.folders.grouping.modes[mode]}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
          <p className="px-2 pt-1 pb-1.5 text-xs text-muted-foreground">
            {t.folders.grouping.autoHint}
          </p>
          {grouped && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onSelect={() => {
                  if (groupId !== undefined) void openTagDialog(groupId);
                }}
              >
                <Tag aria-hidden="true" />
                {t.folders.grouping.toTag}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {tagging !== null && (
        <GroupTagDialog
          group={tagging}
          onClose={() => setTagging(null)}
          onTagged={(result) => {
            setTagging(null);
            toast(taggedMessage(result));
            // Tag conversion dissolves the automatic group and records ungroup for its folder.
            onGroupingChange({ mode: "ungroup" });
            onTagged();
          }}
          onGone={gone}
        />
      )}
    </>
  );
}
