import { Globe } from "lucide-react";
import { useId } from "react";

import type { WorkInfo as WorkInfoValue } from "../../api/workInfo";
import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import WorkInfo from "../WorkInfo";
import { hasWorkItems } from "./videoTabs";

/**
 * The work's items under a header row that holds `Fill in from a site`. The fetch dialog belongs
 * to the tabs container, so a fetch outlives a switch to another tab.
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
  const labelId = useId();
  const filled = hasWorkItems(info);
  const list = filled ? (
    <WorkInfo info={info} />
  ) : (
    <p className="text-sm text-muted-foreground">{t.player.tabs.noWorkDetails}</p>
  );
  if (!canFetch) return list;
  // The action heads the panel at full size, in the same place whether the list is there or not.
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-border pb-3">
        <p id={labelId} className="text-sm text-muted-foreground">
          {t.player.workInfoFetch.entry}
        </p>
        <Button
          variant="secondary"
          shape="pill"
          aria-describedby={labelId}
          onClick={onFetch}
        >
          <Globe aria-hidden="true" />
          {filled ? t.player.tabs.fetchAgain : t.player.tabs.fetch}
        </Button>
      </div>
      <div className="min-w-0">{list}</div>
    </div>
  );
}
