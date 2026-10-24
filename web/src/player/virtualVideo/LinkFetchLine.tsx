import { CircleAlert, LoaderCircle } from "lucide-react";

import type { Video } from "../../api/client";
import { t } from "../../i18n";

/**
 * The owner's line for the fetch a video created from a link started: a running note while the empty fields fill in,
 * so the page stays usable, or why it failed. A finished fetch shows nothing, since its values sit in the fields.
 */
export default function LinkFetchLine({ video }: { video: Video }) {
  const fetch = video.linkFetch;
  const words = t.player.virtual.linkFetch;
  if (fetch === undefined || fetch.state === "done") return null;
  if (fetch.state === "running")
    return (
      <p role="status" className="flex items-center gap-2 text-xs text-muted-foreground">
        <LoaderCircle
          className="size-4 shrink-0 animate-spin text-primary motion-reduce:animate-none"
          aria-hidden="true"
        />
        {words.running}
      </p>
    );
  return (
    <p role="status" className="flex items-start gap-2 text-xs text-muted-foreground">
      <CircleAlert className="size-4 shrink-0 text-destructive" aria-hidden="true" />
      <span>
        {words.failed}
        {fetch.failure !== undefined && <> {fetch.failure}</>} {words.failedNext}
      </span>
    </p>
  );
}
