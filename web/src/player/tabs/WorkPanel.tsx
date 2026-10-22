import { CloudDownload } from "lucide-react";

import type { WorkInfo as WorkInfoValue } from "../../api/workInfo";
import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import WorkInfo from "../WorkInfo";
import { hasWorkItems } from "./videoTabs";

/**
 * The work's items and `Fill in from a site`. The fetch dialog belongs to the tabs container, so a
 * fetch outlives a switch to another tab.
 */
export default function WorkPanel({
  info,
  canFetch,
  onFetch,
}: {
  info: WorkInfoValue | undefined;
  /** Whether an approved work-info plugin exists. */
  canFetch: boolean;
  onFetch: () => void;
}) {
  return (
    <div className="flex flex-col gap-6">
      {hasWorkItems(info) ? (
        <WorkInfo info={info} />
      ) : (
        // The action stays in the same place whether the list is there or not.
        <p className="text-sm text-muted-foreground">{t.player.tabs.noWorkDetails}</p>
      )}
      {canFetch && (
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={onFetch}>
            <CloudDownload aria-hidden="true" />
            {t.player.workInfoFetch.entry}
          </Button>
        </div>
      )}
    </div>
  );
}
