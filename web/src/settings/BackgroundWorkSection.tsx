import { Pause } from "lucide-react";

import type { BackgroundActivity, BackgroundWork } from "../api/background";
import type { ScanActivityKind } from "../api/client";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { BackgroundCounts } from "../shell/BackgroundCounts";
import { type BackgroundSource, useBackground } from "../shell/BackgroundProvider";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
import { Badge } from "../ui/shadcn/badge";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";

import BackgroundPauseSwitch from "./BackgroundPauseSwitch";
import { useSectionAnchor } from "./useSectionAnchor";

const anchorId = "background-work";

/** Scan and import entries carry the import stage kinds, in the job's spelling. */
const stageKinds: Record<string, ScanActivityKind> = {
  registering: "registering",
  probe: "probe",
  thumbnail: "thumbnail",
  seek_thumbnail: "seekThumbnail",
  preview: "preview",
  fingerprint: "fingerprint",
};

function entryAction(entry: BackgroundActivity): UiText {
  const text = t.shell.background;
  if (entry.group === "scan" || entry.group === "import") {
    const stage = stageKinds[entry.kind];
    return stage === undefined ? text.groups[entry.group] : t.shell.scan.activity[stage];
  }
  return text.groupAction[entry.group];
}

/** `{action} · {video title or file name}`, or the action alone; after a pause, `Stopping · …`. */
function entryText(entry: BackgroundActivity, stopping: boolean): UiText {
  const action = stopping ? t.shell.background.stopping : entryAction(entry);
  const subject = entry.video?.title ?? entry.fileName;
  return subject === undefined || subject === ""
    ? action
    : t.shell.scan.activityLine(action, subject);
}

function StateBadge({ status }: { status: BackgroundWork }) {
  const text = t.settings.backgroundWork.state;
  if (status.paused) {
    return (
      <Badge variant="warning">
        <Pause aria-hidden="true" />
        {text.paused}
      </Badge>
    );
  }
  if (status.running.length > 0) {
    return (
      <Badge variant="secondary">
        <Spinner aria-hidden="true" />
        {text.running}
      </Badge>
    );
  }
  return <Badge variant="secondary">{text.idle}</Badge>;
}

function RunningList({ status }: { status: BackgroundWork }) {
  const text = t.settings.backgroundWork;
  return (
    <div className="flex flex-col gap-1">
      <p className="text-xs text-muted-foreground">{text.runningLabel}</p>
      {status.running.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {status.paused ? text.nothingRunsWhilePaused : text.nothingRunning}
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {status.running.map((entry, index) => {
            const line = entryText(entry, status.paused);
            return (
              // The list has no stable id; entries keep their order, oldest first.
              <li
                key={index}
                title={line}
                className={cn(
                  "flex min-w-0 items-center gap-2 text-sm",
                  status.paused && "text-muted-foreground",
                )}
              >
                {!status.paused && <Spinner aria-hidden="true" className="shrink-0" />}
                <span className="truncate">{line}</span>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function LoadingRows() {
  const text = t.settings.backgroundWork;
  return (
    <div role="status" aria-label={text.loading} className="flex flex-col gap-3">
      <Skeleton className="h-5 w-full" />
      {/* The six count rows, each the height of a divided fact row. */}
      <div className="flex flex-col">
        {Array.from({ length: 6 }, (_, index) => (
          <Skeleton key={index} className="my-1 h-6 w-full" />
        ))}
      </div>
    </div>
  );
}

function SectionRows({ source }: { source: BackgroundSource }) {
  const text = t.settings.backgroundWork;
  const status = source.settings;
  if (status === null && source.loadError !== null) {
    return (
      <div>
        <ErrorState
          title={text.loadFailed(source.loadError)}
          retryLabel={t.common.retry}
          onRetry={() => void source.load()}
        />
      </div>
    );
  }
  if (status === null) return <LoadingRows />;
  return (
    <>
      <BackgroundPauseSwitch
        source={source}
        status={status}
        id="background-pause"
        description={text.switchDescription}
      />
      <RunningList status={status} />
      <BackgroundCounts groups={status.groups} />
    </>
  );
}

/** The first section of the Settings Scan page; a guest gets no section and no status read. */
export default function BackgroundWorkSection() {
  const anchor = useSectionAnchor(anchorId);
  const source = useBackground();
  if (source === null) return null;
  const status = source.settings;
  return (
    <div ref={anchor} id={anchorId}>
      <PageSection
        title={t.settings.backgroundWork.heading}
        description={
          status !== null ? (
            <StateBadge status={status} />
          ) : source.loadError === null ? (
            <Skeleton asChild className="inline-block h-6 w-16">
              <span />
            </Skeleton>
          ) : undefined
        }
      >
        <SectionRows source={source} />
      </PageSection>
    </div>
  );
}
