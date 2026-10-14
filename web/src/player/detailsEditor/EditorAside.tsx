import { ArrowLeft, Film } from "lucide-react";
import { type MouseEvent, type Ref, useState } from "react";
import { Link } from "react-router";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { formatBytes, formatDuration, formatResolution } from "../../lib/format";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";

/** fileName はファイルの名前（所在のパスの最後）で、所在が無ければ元の題名である。 */
function fileName(video: Video): string {
  const path = video.location?.path;
  if (path !== undefined) {
    const name = path.split(/[\\/]/).at(-1);
    if (name !== undefined && name !== "") return name;
  }
  return video.fileTitle ?? video.title;
}

/** folderPath は、ファイルを置いたフォルダを登録フォルダの名前から書いたものである。 */
function folderPath(video: Video): string | undefined {
  const folder = video.folder;
  if (folder?.rootName === undefined) return undefined;
  return folder.path === "" ? folder.rootName : `${folder.rootName}/${folder.path}`;
}

/** facts は「{長さ} · {サイズ} · {解像度}」で、分からない値は並べない。 */
function facts(video: Video): string {
  const values = [
    formatDuration(video.durationMs),
    formatBytes(video.sizeBytes),
    video.probeState === "done" ? formatResolution(video) : "",
  ];
  return values.filter((value) => value !== "").join(" · ");
}

/**
 * EditorAside は編集画面の左の列の中身で、どの動画を編集しているかを示す
 * （ui-design.md「Screen」の Left column）。サムネイル、ファイル名、長さ・サイズ・解像度、
 * 置き場所、「Back to the video」。どれも編集できない。`video` が無い間は形だけを出す。
 */
export default function EditorAside({
  video,
  backTo,
  onBack,
  backRef,
}: {
  video: Video | undefined;
  backTo: string;
  /** 「Back to the video」を押した。移るかは呼び手が決める（Cancel と同じ）。 */
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
            // 移るのは、下書きが変わっていないか「Discard」を選んだときだけ（Leaving）。
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
