import { ArrowLeft, Film } from "lucide-react";
import { type MouseEvent, type Ref, useState } from "react";
import { Link } from "react-router";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { formatBytes, formatDuration, formatResolution } from "../../lib/format";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";

function fileName(video: Video): string {
  const path = video.location?.path;
  if (path !== undefined) {
    const name = path.split(/[\\/]/).at(-1);
    if (name !== undefined && name !== "") return name;
  }
  return video.fileTitle ?? video.title;
}

function folderPath(video: Video): string | undefined {
  const folder = video.folder;
  if (folder?.rootName === undefined) return undefined;
  return folder.path === "" ? folder.rootName : `${folder.rootName}/${folder.path}`;
}

function facts(video: Video): string {
  const values = [
    formatDuration(video.durationMs),
    formatBytes(video.sizeBytes),
    video.probeState === "done" ? formatResolution(video) : "",
  ];
  return values.filter((value) => value !== "").join(" · ");
}

export default function EditorAside({
  video,
  backTo,
  onBack,
  backRef,
}: {
  video: Video | undefined;
  backTo: string;
  onBack: () => void;
  backRef?: Ref<HTMLAnchorElement>;
}) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  const url = video?.thumbnailUrl;
  const folder = video === undefined ? undefined : folderPath(video);
  return (
    <>
      {video === undefined ? (
        <Skeleton className="aspect-video w-full max-w-form-page-aside rounded-md" />
      ) : url !== undefined && url !== brokenUrl ? (
        <img
          src={url}
          alt=""
          onError={() => setBrokenUrl(url)}
          className="aspect-video w-full max-w-form-page-aside rounded-md bg-card object-cover"
        />
      ) : (
        <div className="flex aspect-video w-full max-w-form-page-aside items-center justify-center rounded-md bg-card text-muted-foreground">
          <Film className="size-6" aria-hidden="true" />
        </div>
      )}
      {video === undefined ? (
        <>
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-4 w-1/2" />
        </>
      ) : (
        <>
          <p className="font-medium wrap-anywhere">{fileName(video)}</p>
          <p className="text-muted-foreground tabular-nums">{facts(video)}</p>
          {folder !== undefined && (
            <p className="text-muted-foreground wrap-anywhere">{folder}</p>
          )}
        </>
      )}
      <Button
        asChild
        variant="link"
        size="sm"
        className="self-start px-0 text-muted-foreground hover:text-primary has-[>svg]:px-0"
      >
        <Link
          ref={backRef}
          to={backTo}
          onClick={(event: MouseEvent) => {
            // Navigate only when unchanged or after explicit discard.
            event.preventDefault();
            onBack();
          }}
        >
          <ArrowLeft aria-hidden="true" />
          {t.player.detailsEditor.back}
        </Link>
      </Button>
    </>
  );
}
