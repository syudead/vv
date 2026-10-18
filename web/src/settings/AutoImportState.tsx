import { TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";

import type { AutoImportSettings } from "../api/client";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { ErrorState } from "../ui/patterns/error-state";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Skeleton } from "../ui/shadcn/skeleton";

import { PageLinkText } from "./PageLink";
import type { AutoImportSource } from "./useAutoImportSettings";

export function autoImportStateLine(settings: AutoImportSettings): ReactNode {
  const text = t.settings.autoImport.state;
  if (!settings.enabled) return <PageLinkText sentence={text.off} page="library" />;
  switch (settings.watch.state) {
    case "off":
      return <PageLinkText sentence={text.noFolder} page="library" />;
    case "starting":
      return text.starting;
    case "active":
      return text.active;
    case "limited":
      return text.limited;
  }
}

function ProblemAlert({ watch }: { watch: AutoImportSettings["watch"] }) {
  if (watch.state !== "limited" || watch.problem === undefined) return null;
  const text = t.settings.autoImport;
  const problem = text.problem[watch.problem];
  // Pathless events_lost messages are already complete.
  const path = watch.problem === "events_lost" ? null : (watch.path ?? text.unknownPath);
  return (
    <div>
      <Alert variant="warning" role="note">
        <TriangleAlert aria-hidden="true" />
        <AlertTitle>{problem.title}</AlertTitle>
        <AlertDescription>
          <p>
            {problem.before}
            {path !== null &&
              (watch.path !== undefined ? (
                <code className="break-all">{path}</code>
              ) : (
                path
              ))}
            {problem.after}
          </p>
        </AlertDescription>
      </Alert>
    </div>
  );
}

/** Pass a source configured with followWatch to track watcher lifecycle changes. */
export default function AutoImportState({ source }: { source: AutoImportSource }) {
  const text = t.settings.autoImport;
  const { settings, loadError } = source;
  if (settings === null) {
    if (loadError !== null)
      return (
        <div>
          <ErrorState
            title={text.loadFailed(loadError)}
            retryLabel={t.common.retry}
            onRetry={() => void source.load()}
          />
        </div>
      );
    return <Skeleton role="status" aria-label={text.loading} className="h-5" />;
  }
  return (
    <>
      <p
        data-slot="auto-import-state"
        className={cn("text-sm", !settings.enabled && "text-muted-foreground")}
      >
        {autoImportStateLine(settings)}
      </p>
      <ProblemAlert watch={settings.watch} />
    </>
  );
}
