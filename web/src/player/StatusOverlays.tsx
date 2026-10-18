import {
  AlertCircle,
  AlertTriangle,
  Circle,
  CircleCheck,
  FolderOpen,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  WifiOff,
} from "lucide-react";
import { useState, type ReactNode } from "react";

import type { Video } from "../api/client";
import { probeErrorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration } from "../lib/format";
import { Button } from "../ui/shadcn/button";
import type { PlaybackFailureKind } from "./playbackRecovery";
import { useOpenFile } from "./VideoFacts";
import { creatingLine, processingStages, type StageState } from "./processing";

/** Keep status text on an opaque panel within the player. */

const spin = "animate-spin motion-reduce:animate-none";
const fadeIn = "animate-fade-in motion-reduce:animate-none";

function Panel({
  children,
  className,
  role,
}: {
  children: ReactNode;
  className?: string;
  role?: "alert" | "status";
}) {
  return (
    <div
      role={role}
      className={cn(
        "pointer-events-auto flex w-full flex-col rounded-lg bg-navbar p-5 shadow-elevated",
        className,
      )}
    >
      {children}
    </div>
  );
}

/** Let content exceed the 16:9 minimum when the player is absent. */
function Surface({ children }: { children: ReactNode }) {
  return (
    <div
      className={cn(
        "flex w-full items-center justify-center bg-navbar p-4 pt-16 sm:px-6 sm:pb-6 lg:p-6",
        fadeIn,
      )}
    >
      {children}
    </div>
  );
}

/** Reserve space beneath the panel for controls, which remain above the overlay. */
export function Dimmed({ children }: { children: ReactNode }) {
  return (
    <div
      className={cn(
        "pointer-events-auto flex w-full items-center justify-center bg-overlay px-4 pt-16 pb-control-bar-clearance sm:px-6 lg:pt-6",
        fadeIn,
      )}
    >
      {children}
    </div>
  );
}

export function LoadingOverlay({
  backdrop,
  label = t.player.loading,
}: {
  backdrop: boolean;
  label?: UiText;
}) {
  return (
    <div
      // The noninteractive loading indicator lets pointer input reach the seek bar.
      className={cn(
        "pointer-events-none flex w-full items-center justify-center",
        backdrop && "bg-navbar",
      )}
    >
      <span
        role="status"
        className="flex items-center gap-2 rounded-md bg-navbar px-3 py-2 text-sm font-medium text-foreground shadow-elevated"
      >
        <LoaderCircle className={cn("size-5 text-primary", spin)} aria-hidden="true" />
        {label}
      </span>
    </div>
  );
}

export function PlaybackFailure({
  positionMs,
  kind,
  onRetry,
}: {
  positionMs: number;
  kind: PlaybackFailureKind;
  onRetry: () => void;
}) {
  const text = t.player.playbackFailed[kind];
  const Icon = kind === "network" ? WifiOff : AlertCircle;
  return (
    <Dimmed>
      <Panel role="alert" className="max-w-md items-center gap-3 text-center">
        <Icon className="size-8 text-destructive" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-foreground">{text.title}</h2>
        <p className="text-sm text-balance text-muted-foreground">{text.description}</p>
        <Button variant="secondary" onClick={onRetry} className="mt-1">
          <RotateCcw aria-hidden="true" />
          {t.player.playbackFailed.retryFrom(formatDuration(positionMs))}
        </Button>
      </Panel>
    </Dimmed>
  );
}

const stageIcons: Record<StageState, ReactNode> = {
  done: <CircleCheck className="size-4 shrink-0 text-success" aria-hidden="true" />,
  active: (
    <LoaderCircle
      className={cn("size-4 shrink-0 text-primary", spin)}
      aria-hidden="true"
    />
  ),
  waiting: (
    <Circle className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
  ),
  failed: <AlertCircle className="size-4 shrink-0 text-warning" aria-hidden="true" />,
};

export function ProcessingStages({ video }: { video: Video }) {
  return (
    <Surface>
      <div className="flex w-full max-w-sm flex-col gap-3 text-left">
        <h2 className="text-lg font-semibold text-foreground">{t.player.stages.title}</h2>
        <p className="hidden text-sm text-muted-foreground sm:block">
          {t.player.stages.description}
        </p>
        <div role="status" aria-live="polite">
          <ol className="flex flex-col gap-1.5 sm:gap-3">
            {processingStages(video).map((stage) => (
              <li key={stage.id} className="flex items-center gap-2">
                {stageIcons[stage.state]}
                <span
                  className={cn(
                    "min-w-0 flex-1 text-sm",
                    stage.state === "active" && "font-medium text-foreground",
                    stage.state === "done" && "text-foreground",
                    (stage.state === "waiting" || stage.state === "failed") &&
                      "text-muted-foreground",
                  )}
                >
                  {stage.name}
                </span>
                <span
                  className={cn(
                    "shrink-0 text-xs",
                    stage.state === "active" ? "text-primary" : "text-muted-foreground",
                  )}
                >
                  {t.player.stages.states[stage.state]}
                </span>
              </li>
            ))}
          </ol>
        </div>
        <p className="hidden text-xs text-muted-foreground sm:block">
          {t.player.stages.note}
        </p>
      </div>
    </Surface>
  );
}

/** Resolve reprobe only after accepting 202 or probe_not_failed and reloading the video. Keep the button disabled until the processing state appears. */
export function ReadFailure({
  video,
  onReprobe,
}: {
  video: Video;
  /** Omit for guests to hide retry, file actions, and error details. */
  onReprobe?: () => Promise<void>;
}) {
  return (
    <Surface>
      <Panel role="alert" className="max-w-lg gap-3">
        <AlertTriangle className="size-8 text-warning" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-foreground">
          {t.player.readFailure.title}
        </h2>
        {/* Derive descriptions from codes; raw probe errors may contain ffprobe output or legacy localized text. */}
        <p className="text-sm text-muted-foreground">
          {probeErrorText(video.probeErrorCode)}
        </p>
        {onReprobe !== undefined && (
          <ReadFailureActions video={video} onReprobe={onReprobe} />
        )}
      </Panel>
    </Surface>
  );
}

function ReadFailureActions({
  video,
  onReprobe,
}: {
  video: Video;
  onReprobe: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const { open, failure: openFailure } = useOpenFile(video.id);
  const openable = video.location?.openable === true;

  const reprobe = () => {
    setBusy(true);
    setFailed(false);
    void onReprobe()
      .catch(() => setFailed(true))
      .finally(() => setBusy(false));
  };

  return (
    <>
      <div className="mt-1 flex flex-wrap gap-2">
        <Button variant="secondary" onClick={reprobe} disabled={busy}>
          {busy ? (
            <LoaderCircle className={spin} aria-hidden="true" />
          ) : (
            <RefreshCw aria-hidden="true" />
          )}
          {t.player.readFailure.reprobe}
        </Button>
        {openable && (
          <Button variant="secondary" onClick={open}>
            <FolderOpen aria-hidden="true" />
            {t.player.readFailure.openFile}
          </Button>
        )}
      </div>
      {failed && (
        <p className="text-sm text-destructive">{t.player.readFailure.reprobeFailed}</p>
      )}
      {openFailure !== null && <p className="text-sm text-destructive">{openFailure}</p>}
    </>
  );
}

/** The header close button handles returning from a missing video. */
export function MissingVideo() {
  return (
    <Surface>
      <Panel role="alert" className="max-w-md items-center gap-3 text-center">
        <AlertCircle className="size-8 text-muted-foreground" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-foreground">
          {t.player.missing.title}
        </h2>
        <p className="text-sm text-balance text-muted-foreground">
          {t.player.missing.description}
        </p>
      </Panel>
    </Surface>
  );
}

export function LoadFailure({
  reason,
  onRetry,
}: {
  reason: UiText;
  onRetry: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const retry = () => {
    setBusy(true);
    void onRetry().finally(() => setBusy(false));
  };
  return (
    <Surface>
      <Panel role="alert" className="max-w-md items-center gap-3 text-center">
        <AlertCircle className="size-8 text-destructive" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-foreground">{t.player.loadFailed}</h2>
        <p className="text-sm text-balance text-muted-foreground">{reason}</p>
        <Button variant="secondary" onClick={retry} disabled={busy} className="mt-1">
          {busy ? (
            <LoaderCircle className={spin} aria-hidden="true" />
          ) : (
            <RefreshCw aria-hidden="true" />
          )}
          {t.common.retry}
        </Button>
      </Panel>
    </Surface>
  );
}

export function Unplayable() {
  return (
    <Surface>
      <Panel role="alert" className="max-w-md items-center gap-3 text-center">
        <AlertTriangle className="size-8 text-warning" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-foreground">
          {t.player.unplayable.title}
        </h2>
        <p className="text-sm text-balance text-muted-foreground">
          {t.player.unplayable.description}
        </p>
      </Panel>
    </Surface>
  );
}

export function CreatingLine({ video }: { video: Video }) {
  const line = creatingLine(video);
  if (line === null) return null;
  return (
    <p role="status" className="flex items-center gap-2 text-xs text-muted-foreground">
      <LoaderCircle
        className={cn("size-4 shrink-0 text-primary", spin)}
        aria-hidden="true"
      />
      {line}
    </p>
  );
}
