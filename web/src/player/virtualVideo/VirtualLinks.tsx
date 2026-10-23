import { ExternalLink } from "lucide-react";

import type { WorkInfo } from "../../api/workInfo";
import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import { Description, withoutScheme } from "../WorkInfo";

/**
 * A virtual video has nothing to play, so its links take the main action's form under the title,
 * followed by its description. Both are absent when empty.
 */
export default function VirtualLinks({
  videoId,
  info,
}: {
  videoId: number;
  info: WorkInfo | undefined;
}) {
  const urls = info?.urls ?? [];
  const description = info?.description;
  if (urls.length === 0 && description === undefined) return null;
  return (
    <div className="flex flex-col gap-4">
      {urls.length > 0 && (
        <ul aria-label={t.player.virtual.links} className="flex flex-wrap gap-2">
          {urls.map((url) => (
            <li key={url} className="max-w-full min-w-0">
              <Button asChild variant="outline" className="max-w-full">
                <a href={url} title={url} target="_blank" rel="noreferrer">
                  <ExternalLink aria-hidden="true" />
                  <span className="max-w-xs min-w-0 truncate">{withoutScheme(url)}</span>
                </a>
              </Button>
            </li>
          ))}
        </ul>
      )}
      {/* Keyed by video so expansion does not carry over to the next video. */}
      {description !== undefined && <Description key={videoId} text={description} />}
    </div>
  );
}
