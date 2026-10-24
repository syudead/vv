import { Ellipsis, Merge, Trash2 } from "lucide-react";
import { useState } from "react";

import { deleteVideo, type Video } from "../../api/client";
import GroupConfirmDialog from "../../groups/GroupConfirmDialog";
import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import { useToast } from "../../ui/Toast";
import MergeDialog from "./MergeDialog";

/**
 * A virtual video's rare and irreversible actions, after `Edit` on the title row's action line:
 * merging into a file video and deleting.
 */
export default function VirtualMoreMenu({
  video,
  onStale,
  onDeleted,
  onMerged,
}: {
  video: Video;
  onStale: () => void;
  /** Leave the page after the video is deleted. */
  onDeleted: () => void;
  /** Replace the page with the file video's after a merge. */
  onMerged: (merged: Video) => void;
}) {
  const [deleting, setDeleting] = useState(false);
  const [merging, setMerging] = useState(false);
  const toast = useToast();
  const words = t.player.virtual;
  return (
    <>
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t.common.more}
                className="text-muted-foreground"
              >
                <Ellipsis aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>{t.common.more}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => setMerging(true)}>
            <Merge aria-hidden="true" />
            {words.merge.entry}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setDeleting(true)}>
            <Trash2 aria-hidden="true" />
            {words.delete}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {merging && (
        <MergeDialog
          video={video}
          onClose={() => setMerging(false)}
          onMerged={onMerged}
          onReload={() => {
            setMerging(false);
            onStale();
          }}
        />
      )}
      {deleting && (
        <GroupConfirmDialog
          request={() => deleteVideo(video.id, video.contentVersion ?? "")}
          onDone={() => {
            toast(words.deleted(video.title));
            onDeleted();
          }}
          onClose={() => setDeleting(false)}
          onGone={() => {
            setDeleting(false);
            toast(words.alreadyRemoved(video.title));
            // The re-read answers 404, which shows the page's missing-video layer.
            onStale();
          }}
          title={words.deleteTitle(video.title)}
          description={words.deleteDescription}
          submit={words.deleteAction}
          submitting={words.deleting}
          failed={words.deleteFailed}
          changed={{
            text: words.deleteChanged,
            onReload: () => {
              setDeleting(false);
              onStale();
            },
          }}
        />
      )}
    </>
  );
}
