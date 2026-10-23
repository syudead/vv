import { Check, CircleAlert } from "lucide-react";
import { type FormEvent, useMemo, useRef, useState } from "react";

import { mergeVirtualVideo, RequestFailed, type Video } from "../../api/client";
import { useRootFolders } from "../../api/useFolderListing";
import { rootDisplays, topLevelLocationLabel } from "../../folders/folderPath";
import { useLoadMoreWhenNear } from "../../hooks/useLoadMoreWhenNear";
import { errorText, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { formatDuration } from "../../lib/format";
import { FormDialog } from "../../ui/patterns/form-dialog";
import { LoadMoreRow } from "../../ui/patterns/load-more-row";
import { Alert, AlertAction, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "../../ui/shadcn/command";
import { Spinner } from "../../ui/shadcn/spinner";
import { useToast } from "../../ui/Toast";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
import { useFileVideoSearch } from "./useFileVideoSearch";

const words = t.player.virtual.merge;

/** reload marks a content_changed failure the caller answers by re-reading the page. */
interface Failure {
  text: UiText;
  reload: boolean;
}

export function mergeFailure(error: unknown): Failure {
  if (error instanceof RequestFailed) {
    if (error.status === 404) return { text: words.gone, reload: false };
    if (error.reason === "content_changed") return { text: words.changed, reload: true };
    if (error.reason === "description_too_long")
      return { text: words.tooLong, reload: false };
    if (error.reason === "too_many_urls")
      return { text: words.tooManyUrls, reload: false };
  }
  return { text: words.failed(errorText(error)), reload: false };
}

/**
 * Picks one file video and merges the virtual video into it. Mount only while open; failures keep
 * the dialog and the choice, and the caller leaves the page on success.
 */
export default function MergeDialog({
  video,
  onClose,
  onMerged,
  onReload,
}: {
  video: Video;
  onClose: () => void;
  onMerged: (merged: Video) => void;
  /** Close and re-read the page after content_changed. */
  onReload: () => void;
}) {
  const toast = useToast();
  const [query, setQuery] = useState("");
  const search = useFileVideoSearch(query);
  const [chosen, setChosen] = useState<Video | null>(null);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<Failure | null>(null);
  const locations = useLocations();
  const end = useRef<HTMLDivElement | null>(null);
  useLoadMoreWhenNear(
    end,
    search.hasMore && search.more === "idle" && search.status === "ready",
    search.loadMore,
    search.rows.length,
  );

  // A new search can hide the chosen row, so the choice goes with it: Merge never sends a video
  // the owner no longer sees.
  const changeQuery = (value: string) => {
    setQuery(value);
    setChosen(null);
    setFailure(null);
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (chosen === null || pending) return;
    setPending(true);
    setFailure(null);
    mergeVirtualVideo(video.id, {
      contentVersion: video.contentVersion ?? "",
      videoId: chosen.id,
      videoContentVersion: chosen.contentVersion ?? "",
    }).then(
      (merged) => {
        toast(words.merged(merged.title));
        onMerged(merged);
      },
      (error: unknown) => {
        setPending(false);
        setFailure(mergeFailure(error));
      },
    );
  };

  const reading = search.status === "loading" || search.more === "loading";
  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={words.title}
      description={words.description}
      onSubmit={submit}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? words.submitting : words.submit}
        </>
      }
      cancelLabel={t.common.cancel}
      pending={pending}
      submitDisabled={chosen === null}
    >
      <Command
        label={words.search}
        shouldFilter={false}
        className="rounded-md border border-border bg-transparent"
      >
        <CommandInput
          value={query}
          onValueChange={changeQuery}
          placeholder={words.search}
          aria-label={words.search}
          trailing={reading ? <Spinner className="text-muted-foreground" /> : undefined}
        />
        {search.status === "failed" ? (
          <ReadFailure onRetry={search.retry} />
        ) : (
          <CommandList label={words.list} className="p-1">
            {search.status === "ready" && <CommandEmpty>{words.none}</CommandEmpty>}
            {search.rows.map((row) => (
              <FileVideoRow
                key={row.id}
                video={row}
                location={locations(row)}
                chosen={chosen?.id === row.id}
                disabled={pending}
                onChoose={() => setChosen(row)}
              />
            ))}
            {search.more === "failed" ? (
              <ReadFailure onRetry={search.retry} />
            ) : (
              <div ref={end} />
            )}
          </CommandList>
        )}
      </Command>
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure.text}</AlertTitle>
          {failure.reload && (
            <AlertAction>
              <Button type="button" variant="outline" size="sm" onClick={onReload}>
                {t.common.reload}
              </Button>
            </AlertAction>
          )}
        </Alert>
      )}
    </FormDialog>
  );
}

function ReadFailure({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="p-1">
      <LoadMoreRow
        status="failed"
        title={words.loadFailed}
        retryLabel={t.common.retry}
        onRetry={onRetry}
      />
    </div>
  );
}

function FileVideoRow({
  video,
  location,
  chosen,
  disabled,
  onChoose,
}: {
  video: Video;
  location: string | undefined;
  chosen: boolean;
  disabled: boolean;
  onChoose: () => void;
}) {
  const facts = [formatDuration(video.durationMs), location].filter(
    (fact) => fact !== undefined && fact !== "",
  );
  return (
    <CommandItem
      value={String(video.id)}
      disabled={disabled}
      onSelect={onChoose}
      aria-checked={chosen}
      data-chosen={chosen || undefined}
      className={cn(
        "gap-3",
        chosen &&
          "bg-primary-soft text-primary data-[selected=true]:bg-primary-soft data-[selected=true]:text-primary",
      )}
    >
      <VideoThumbnail className="w-list-thumb shrink-0 rounded-sm">
        {video.thumbnailUrl !== undefined && (
          <VideoThumbnailImage src={video.thumbnailUrl} />
        )}
      </VideoThumbnail>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{video.title}</span>
        {facts.length > 0 && (
          <span className="truncate text-xs text-muted-foreground tabular-nums">
            {facts.join(" · ")}
          </span>
        )}
      </span>
      {chosen && <Check aria-hidden="true" className="text-primary" />}
    </CommandItem>
  );
}

/** Folder labels as search results show them; a root not loaded yet shows the path alone. */
function useLocations(): (video: Video) => string | undefined {
  const roots = useRootFolders();
  const names = useMemo(
    () => rootDisplays(roots.data?.folders ?? []),
    [roots.data?.folders],
  );
  return (video) => {
    const folder = video.folder;
    if (folder === undefined) return undefined;
    const label = topLevelLocationLabel(folder, names.get(folder.rootId));
    return label?.label ?? (folder.path === "" ? undefined : folder.path);
  };
}
