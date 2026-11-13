import {
  ChevronDown,
  CircleAlert,
  CircleCheck,
  File as FileIcon,
  Film,
  type LucideIcon,
  SearchX,
} from "lucide-react";
import { type ReactNode, useState } from "react";

import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import { formatBytes, formatDuration } from "../../lib/format";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import type { ImageSearchPhase } from "./imageSearchState";
import { type FileKind, fileKind, useFilePreview } from "./useFilePreview";

export interface QueryPanelProps {
  phase: ImageSearchPhase;
  /** Below lg, whether the panel shows more than its one-line bar; from lg it always does. */
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Opens the search-by-file dialog. */
  onChoose: () => void;
  /** Ends the running search. */
  onCancel: () => void;
}

interface Status {
  icon: ReactNode;
  label: string;
}

function statusIcon(Icon: LucideIcon, tone: string): ReactNode {
  return <Icon aria-hidden="true" className={cn("size-4 shrink-0", tone)} />;
}

/** The panel's status line by phase: the answer in two words, the reason being in the results column. */
export function panelStatus(phase: ImageSearchPhase): Status {
  const text = t.library.imageSearch;
  switch (phase.kind) {
    case "searching":
      return { icon: <Spinner aria-hidden="true" />, label: text.searching };
    case "results": {
      const count = phase.result.items.length;
      return count > 0
        ? { icon: statusIcon(CircleCheck, "text-success"), label: text.found(count) }
        : {
            icon: statusIcon(SearchX, "text-muted-foreground"),
            label: text.noMatchStatus,
          };
    }
    case "refused":
      return {
        icon: statusIcon(CircleAlert, "text-destructive"),
        label: phase.refusal.retry ? text.searchFailed : text.notSearched,
      };
  }
}

/** The file line's quieter half: its kind, a video's length when known, and its size. */
export function fileFacts(file: File, durationMs: number | undefined): string {
  const kind = fileKind(file);
  const parts: string[] = [t.library.imageSearch.kinds[kind]];
  if (kind === "video" && durationMs !== undefined)
    parts.push(formatDuration(durationMs));
  parts.push(formatBytes(file.size));
  return parts.join(" · ");
}

const previewKinds = ["image", "video"] as const;

function KindIcon({ kind, className }: { kind: FileKind; className: string }) {
  const Icon = kind === "file" ? FileIcon : Film;
  return <Icon aria-hidden="true" className={cn("text-muted-foreground", className)} />;
}

/**
 * The file as large as the panel allows, whole and never cropped: an image, or a muted video with the
 * browser's controls. A file the browser cannot show is an icon on a 16:9 frame.
 */
function Preview({
  file,
  url,
  onDuration,
}: {
  file: File;
  url: string | null;
  onDuration: (durationMs: number) => void;
}) {
  const [broken, setBroken] = useState(false);
  const kind = fileKind(file);
  const size =
    "max-h-query-preview-max-sm w-full rounded-sm bg-muted lg:max-h-query-preview-max";
  if (url === null || broken || kind === "file") {
    return (
      <div
        data-slot="query-preview"
        className="flex aspect-video w-full items-center justify-center rounded-sm bg-muted"
      >
        <KindIcon kind={kind} className="size-8" />
      </div>
    );
  }
  if (kind === "image") {
    return (
      <img
        data-slot="query-preview"
        src={url}
        alt=""
        onError={() => setBroken(true)}
        className={cn(size, "h-auto object-contain")}
      />
    );
  }
  return (
    <video
      data-slot="query-preview"
      src={url}
      controls
      muted
      playsInline
      preload="metadata"
      onLoadedMetadata={(event) => {
        const seconds = event.currentTarget.duration;
        if (Number.isFinite(seconds)) onDuration(Math.round(seconds * 1000));
      }}
      onError={() => setBroken(true)}
      className={size}
    />
  );
}

/** The panel's collapsed form below lg: a small preview, the name, the status and the toggle. */
function PanelBar({
  file,
  url,
  status,
  open,
  onOpenChange,
}: {
  file: File;
  url: string | null;
  status: Status;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const text = t.library.imageSearch;
  const kind = fileKind(file);
  const label = open ? text.hideFile : text.showFile;
  return (
    <div
      data-slot="query-panel-bar"
      className="flex min-w-0 items-center gap-2 lg:hidden"
    >
      <span className="flex aspect-video h-9 shrink-0 items-center justify-center overflow-hidden rounded-sm bg-muted">
        {kind === "image" && url !== null ? (
          <img src={url} alt="" className="size-full object-contain" />
        ) : (
          <KindIcon kind={kind} className="size-4" />
        )}
      </span>
      <span className="min-w-0 flex-1 truncate text-sm font-medium">{file.name}</span>
      <span className="flex shrink-0 items-center gap-1.5 text-sm">
        {status.icon}
        <span>{status.label}</span>
      </span>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            aria-expanded={open}
            onClick={() => onOpenChange(!open)}
          >
            <ChevronDown
              aria-hidden="true"
              className={cn("transition-transform", open && "rotate-180")}
            />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </div>
  );
}

/** The file an image search runs on, beside its results: what it is, the search's state and what to do next. */
export function QueryPanel({
  phase,
  open,
  onOpenChange,
  onChoose,
  onCancel,
}: QueryPanelProps) {
  const text = t.library.imageSearch;
  const { file } = phase;
  const url = useFilePreview(file, previewKinds);
  const [readDuration, setReadDuration] = useState<number>();
  const answered = phase.kind === "results" ? phase.result : undefined;
  const durationMs = answered?.query.durationMs ?? readDuration;
  const status = panelStatus(phase);
  return (
    <div
      data-slot="query-panel"
      className="flex min-w-0 flex-col gap-3 rounded-md border border-border bg-card p-3"
    >
      <PanelBar
        file={file}
        url={url}
        status={status}
        open={open}
        onOpenChange={onOpenChange}
      />
      <div className={cn("flex min-w-0 flex-col gap-3", !open && "hidden lg:flex")}>
        <Preview key={url} file={file} url={url} onDuration={setReadDuration} />
        <div className="flex min-w-0 flex-col gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="truncate text-sm font-medium">{file.name}</span>
            </TooltipTrigger>
            <TooltipContent>{file.name}</TooltipContent>
          </Tooltip>
          <span className="text-xs text-muted-foreground tabular-nums">
            {fileFacts(file, durationMs)}
          </span>
        </div>
        <div role="status" className="flex min-w-0 items-center gap-2">
          {status.icon}
          <span className="min-w-0 flex-1 truncate text-sm font-medium">
            {status.label}
          </span>
        </div>
        {answered !== undefined && answered.unindexedCount > 0 && (
          <p className="text-xs text-muted-foreground">
            {text.unindexed(answered.unindexedCount)}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          {phase.kind === "searching" ? (
            <Button variant="outline" size="sm" onClick={onCancel}>
              {text.cancel}
            </Button>
          ) : (
            <Button variant="outline" size="sm" onClick={onChoose}>
              {text.chooseAnother}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
