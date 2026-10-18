import { useEffect, useRef, useState } from "react";
import { Ellipsis, Folder, FolderMinus, Tag, Trash2 } from "lucide-react";
import { Link } from "react-router";

import { isAborted, RequestFailed, type Video } from "../api/client";
import { findVideoItem, removeFromGroup, taggedMessage } from "../api/groups";
import { clearListSnapshot } from "../api/listSnapshot";
import DissolveGroupDialog from "../groups/DissolveGroupDialog";
import GroupTagDialog from "../groups/GroupTagDialog";
import { groupPagePath } from "../groups/groupPaths";
import { errorText, t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";

type VideoGroup = NonNullable<Video["group"]>;

/** Keep group navigation separate from the owner's actions menu. */
export default function GroupLine({
  group,
  videoId,
  owner,
  backTo,
  onChanged,
}: {
  group: VideoGroup;
  videoId: number;
  owner: boolean;
  /** Preserve the video's return destination as state.from. */
  backTo: string;
  /** Reload the video and related results without group context after mutations. */
  onChanged: () => void;
}) {
  return (
    <div className="flex min-w-0 items-center gap-1">
      <Link
        to={groupPagePath(group.id)}
        state={{ from: backTo }}
        aria-label={t.player.group.line(group.name, group.position, group.count)}
        data-group-line=""
        className="flex max-w-full min-w-0 items-center gap-1.5 rounded-sm text-xs text-muted-foreground hover:text-foreground sm:text-sm"
      >
        <Folder className="size-4 shrink-0" aria-hidden="true" />
        <span className="truncate" title={group.name}>
          {group.name}
        </span>
        <span className="shrink-0" aria-hidden="true">
          ·
        </span>
        <span className="shrink-0 tabular-nums">
          {t.player.related.position(group.position, group.count)}
        </span>
      </Link>
      {owner && (
        <GroupActions
          key={`${String(group.id)}:${String(videoId)}`}
          group={group}
          videoId={videoId}
          onChanged={onChanged}
        />
      )}
    </div>
  );
}

/** Return the direct item's ID, or undefined while loading, missing, or failed. */
function useDirectItem(groupId: number, videoId: number): number | undefined {
  const [itemId, setItemId] = useState<number | undefined>(undefined);
  useEffect(() => {
    const controller = new AbortController();
    findVideoItem(groupId, videoId, controller.signal).then(
      setItemId,
      (error: unknown) => {
        if (!isAborted(error)) setItemId(undefined);
      },
    );
    return () => controller.abort();
  }, [groupId, videoId]);
  return itemId;
}

function GroupActions({
  group,
  videoId,
  onChanged,
}: {
  group: VideoGroup;
  videoId: number;
  onChanged: () => void;
}) {
  const toast = useToast();
  const itemId = useDirectItem(group.id, videoId);
  const [dialog, setDialog] = useState<"tag" | "dissolve" | null>(null);
  const [removing, setRemoving] = useState(false);
  // Ignore responses after navigation; reloading would send the user back to the old video.
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const changed = () => {
    if (mounted.current) onChanged();
  };

  const remove = (id: number) => {
    setRemoving(true);
    removeFromGroup(group.id, [id]).then(
      () => {
        setRemoving(false);
        clearListSnapshot();
        toast(t.player.group.removed(group.name));
        changed();
      },
      (error: unknown) => {
        setRemoving(false);
        // A missing item or group requires a silent refresh of membership.
        if (error instanceof RequestFailed && error.status === 404) {
          clearListSnapshot();
          changed();
          return;
        }
        toast(t.player.group.removeFailed(errorText(error)));
      },
    );
  };

  const gone = () => {
    setDialog(null);
    changed();
  };

  return (
    <>
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={removing}
                aria-label={t.player.group.actions}
                className="shrink-0 text-muted-foreground"
              >
                <Ellipsis aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>{t.player.group.actions}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="start">
          {itemId !== undefined && (
            <DropdownMenuItem onSelect={() => remove(itemId)}>
              <FolderMinus aria-hidden="true" />
              {t.player.group.remove}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={() => setDialog("tag")}>
            <Tag aria-hidden="true" />
            {t.player.group.toTag}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setDialog("dissolve")}>
            <Trash2 aria-hidden="true" />
            {t.player.group.dissolve}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog === "tag" && (
        <GroupTagDialog
          group={group}
          onClose={() => setDialog(null)}
          onTagged={(result) => {
            setDialog(null);
            toast(taggedMessage(result));
            changed();
          }}
          onGone={gone}
        />
      )}
      {dialog === "dissolve" && (
        <DissolveGroupDialog
          group={group}
          onClose={() => setDialog(null)}
          onDissolved={() => {
            setDialog(null);
            toast(t.groups.dissolved(group.name));
            changed();
          }}
          onGone={gone}
        />
      )}
    </>
  );
}
