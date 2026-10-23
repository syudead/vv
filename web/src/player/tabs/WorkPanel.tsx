import { useId } from "react";

import type { WorkInfo as WorkInfoValue } from "../../api/workInfo";
import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import WorkInfo from "../WorkInfo";
import { PanelColumns, PanelLabel } from "./panelParts";
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
  const labelId = useId();
  const filled = hasWorkItems(info);
  const list = filled ? (
    <WorkInfo info={info} />
  ) : (
    <p className="text-sm text-muted-foreground">{t.player.tabs.noWorkDetails}</p>
  );
  if (!canFetch) return list;
  // The action keeps its column whether the list is there or not.
  return (
    <PanelColumns
      main={list}
      side={
        <div className="flex flex-col items-start gap-2">
          <PanelLabel id={labelId}>{t.player.workInfoFetch.entry}</PanelLabel>
          <Button
            variant="secondary"
            size="sm"
            shape="pill"
            aria-describedby={labelId}
            onClick={onFetch}
          >
            {filled ? t.player.tabs.fetchAgain : t.player.tabs.fetch}
          </Button>
        </div>
      }
    />
  );
}
