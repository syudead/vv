import type { BackgroundGroup } from "../api/background";
import { t } from "../i18n";
import { FactList } from "../ui/patterns/fact-list";
import { Badge } from "../ui/shadcn/badge";

function GroupName({ group }: { group: BackgroundGroup }) {
  const text = t.shell.background;
  return (
    <span className="flex items-center gap-2">
      {text.groups[group.group]}
      {group.waiting === true && <span>{text.waitingForResume}</span>}
      {group.featureOff === true && <Badge variant="secondary">{text.featureOff}</Badge>}
    </span>
  );
}

function GroupValue({ group }: { group: BackgroundGroup }) {
  // A waiting scan with nothing counted yet, or everything read, says only that it waits.
  if (group.waiting === true && group.remaining === 0) return null;
  const value = t.shell.background.remaining[group.group](group.remaining);
  return group.remaining === 0 ? (
    <span className="text-muted-foreground">{value}</span>
  ) : (
    value
  );
}

/**
 * The work left per group, one row each in the order the status lists them; the Settings section
 * and the indicator's summary both draw it so they never disagree.
 */
export function BackgroundCounts({ groups }: { groups: readonly BackgroundGroup[] }) {
  const text = t.shell.background;
  return (
    <div className="flex flex-col gap-1">
      <p className="text-xs text-muted-foreground">{text.countsLabel}</p>
      <FactList
        variant="divided"
        aria-label={text.countsLabel}
        facts={groups.map((group) => ({
          id: group.group,
          term: <GroupName group={group} />,
          value: <GroupValue group={group} />,
        }))}
      />
      {groups.some((group) => group.featureOff === true) && (
        <p className="text-xs text-muted-foreground">{text.featureOffNote}</p>
      )}
    </div>
  );
}
