import {
  CalendarPlus,
  Ellipsis,
  ImageUp,
  LoaderCircle,
  PencilLine,
  Trash2,
} from "lucide-react";
import { useState } from "react";

import { deleteVideo, type Video } from "../../api/client";
import type { DetailMark } from "../../api/useVideoDetail";
import type { WorkInfo as WorkInfoValue } from "../../api/workInfo";
import GroupConfirmDialog from "../../groups/GroupConfirmDialog";
import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import { useToast } from "../../ui/Toast";
import ActionButton from "../ActionButton";
import DetailsEntry, { type DetailsEntryProps } from "../detailsEditor/DetailsEntry";
import {
  ImageFileInput,
  UploadPreviewDialog,
  usePreviewEditing,
} from "../thumbnailUpload/UploadEntry";
import {
  DateFact,
  FactsFailure,
  factsList,
  ImageThumbnailFact,
  ListActions,
  useThumbnailPosition,
} from "../VideoFacts";
import WorkInfo from "../WorkInfo";
import FetchEntry from "../workInfoFetch/FetchEntry";
import type { VirtualThumbnail } from "./useVirtualThumbnail";

/** The links and description above already show these. */
function withoutLinks(info: WorkInfoValue | undefined): WorkInfoValue | undefined {
  if (info === undefined) return undefined;
  return { ...info, description: undefined, urls: [] };
}

/**
 * The facts row of a virtual video: dates and the uploaded image, with no length, size or file
 * actions. A guest sees `Added` only.
 */
export default function VirtualFacts({
  video,
  owner,
  thumbnail,
  onChanged,
  onStale,
  onFavorite,
  onDeleted,
  onEditingChange,
  details,
}: {
  video: Video;
  owner: boolean;
  thumbnail: VirtualThumbnail;
  onChanged: (video: Video, mark: DetailMark) => void;
  onStale: () => void;
  onFavorite: (favorite: boolean) => Promise<void>;
  /** Leave the page after the video is deleted. */
  onDeleted: () => void;
  onEditingChange: (open: boolean) => void;
  details: DetailsEntryProps["details"];
}) {
  const image = useThumbnailPosition(video.id, thumbnail.report, onChanged, onStale);
  usePreviewEditing(thumbnail.upload.preview !== null, onEditingChange);
  const f = t.player.facts;
  return (
    <div className="flex flex-col gap-3 border-t border-border pt-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <ul aria-label={f.label} className={factsList}>
          <DateFact icon={CalendarPlus} label={f.added} value={video.addedAt} />
          {owner && (
            <DateFact icon={PencilLine} label={f.edited} value={video.updatedAt} />
          )}
          {owner && video.thumbnailImage !== undefined && (
            <ImageThumbnailFact
              origin={video.thumbnailImage}
              url={video.thumbnailUrl}
              reverting={image.sending === "revert"}
              locked={thumbnail.busy}
              revertLabel={t.player.virtual.removeImage}
              onRevert={() => image.send("revert")}
            />
          )}
        </ul>
        {owner && (
          <div className="ml-auto flex shrink-0 items-center">
            <ListActions
              video={video}
              favorite={video.favorite ?? false}
              onFavorite={async (next) => {
                thumbnail.report(null);
                await onFavorite(next);
              }}
              onFailed={(reason) => thumbnail.report(f.favoriteFailed(reason))}
            />
            <DetailsEntry video={video} owner={owner} details={details} />
            <FetchEntry
              video={video}
              onApplied={onChanged}
              onDialogChange={onEditingChange}
            />
            <ActionButton
              label={t.player.thumbnailUpload.entry}
              aria-disabled={thumbnail.busy || image.sending !== null || undefined}
              aria-busy={thumbnail.upload.sending || undefined}
              className="aria-disabled:cursor-default"
              onClick={() => {
                if (image.sending === null) thumbnail.pick();
              }}
            >
              {thumbnail.busy ? (
                <LoaderCircle
                  aria-hidden="true"
                  className="animate-spin motion-reduce:animate-none"
                />
              ) : (
                <ImageUp aria-hidden="true" />
              )}
            </ActionButton>
            <MoreMenu video={video} onStale={onStale} onDeleted={onDeleted} />
            <ImageFileInput
              inputRef={thumbnail.fileInput}
              onFile={thumbnail.upload.choose}
            />
            <UploadPreviewDialog video={video} upload={thumbnail.upload} framingOnly />
          </div>
        )}
      </div>
      <FactsFailure failure={thumbnail.failure} />
      <WorkInfo info={withoutLinks(video.workInfo)} />
    </div>
  );
}

/** Rare and irreversible actions sit one step below the actions row. */
function MoreMenu({
  video,
  onStale,
  onDeleted,
}: {
  video: Video;
  onStale: () => void;
  onDeleted: () => void;
}) {
  const [deleting, setDeleting] = useState(false);
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
          <DropdownMenuItem variant="destructive" onSelect={() => setDeleting(true)}>
            <Trash2 aria-hidden="true" />
            {words.delete}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
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
