import { CircleAlert, CircleCheck, LoaderCircle } from "lucide-react";
import { useId } from "react";

import type { FaceRecognitionSettings } from "../api/client";
import { t } from "../i18n";
import { formatBytes } from "../lib/format";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Progress } from "../ui/shadcn/progress";

type FaceRuntime = FaceRecognitionSettings["runtime"];

function DownloadingLine({ runtime }: { runtime: FaceRuntime }) {
  const text = t.settings.faceRecognition;
  const lineId = useId();
  const done = runtime.bytesDone ?? 0;
  const total = runtime.bytesTotal ?? 0;
  return (
    <div className="flex flex-col gap-2">
      <p id={lineId} className="text-xs text-muted-foreground">
        {/* Only the sentence is live, so the byte counts are not announced every second. */}
        <span role="status">{text.downloading}</span>{" "}
        <span className="whitespace-nowrap">
          {text.downloadedBytes(formatBytes(done), formatBytes(total))}
        </span>
      </p>
      <Progress value={done} max={total > 0 ? total : 100} aria-labelledby={lineId} />
    </div>
  );
}

/**
 * The runtime state under the face recognition switch; absent and unsupported draw nothing,
 * since the page notice covers unsupported.
 */
export function FaceRuntimeLine({
  runtime,
  retrying,
  locked,
  onRetry,
}: {
  runtime: FaceRuntime;
  retrying: boolean;
  locked: boolean;
  onRetry: () => void;
}) {
  const text = t.settings.faceRecognition;
  switch (runtime.state) {
    case "downloading":
      return <DownloadingLine runtime={runtime} />;
    case "ready":
      return (
        <p
          role="status"
          className="flex items-center gap-2 text-xs text-muted-foreground"
        >
          <CircleCheck aria-hidden="true" className="size-4" />
          {text.ready}
        </p>
      );
    case "failed":
      return (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{text.failedTitle}</AlertTitle>
          {/* Under the reason, as ErrorState places Retry: AlertAction's corner is too narrow for it. */}
          <AlertDescription className="flex flex-col items-start gap-2">
            {runtime.reason !== undefined && <p>{text.failedReason[runtime.reason]}</p>}
            <Button
              size="sm"
              variant="outline"
              aria-busy={retrying || undefined}
              aria-disabled={retrying || locked || undefined}
              onClick={onRetry}
            >
              {retrying && (
                <LoaderCircle
                  aria-hidden="true"
                  className="animate-spin motion-reduce:animate-none"
                />
              )}
              {text.downloadAgain}
            </Button>
          </AlertDescription>
        </Alert>
      );
    default:
      return null;
  }
}
