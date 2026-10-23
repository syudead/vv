import type { Video } from "../../api/client";
import type { WorkInfo } from "../../api/workInfo";
import { creditDisplayName } from "../../credits/displayName";
import { t } from "../../i18n";
import { formatDuration, formatResolution } from "../../lib/format";
import { type Fact, FactList } from "../../ui/patterns/fact-list";
import { Description } from "../WorkInfo";
import { DateValue, PanelColumns } from "./panelParts";

/** The code, the release date and the performers, each left out when missing. */
export function workSummary(info: WorkInfo | undefined): string {
  if (info === undefined) return "";
  const release = info.dates.find((date) => date.kind === "release");
  // By person ID: two people may share a name, and the People tab counts IDs.
  const performers = [
    ...new Map(
      info.people
        .filter((entry) => entry.role === "performer")
        .map((entry) => [entry.person.id, creditDisplayName(entry.person)]),
    ).values(),
  ];
  return [
    info.code,
    release === undefined ? undefined : t.player.tabs.released(release.date),
    performers.length === 0 ? undefined : performers.join(", "),
  ]
    .filter((part) => part !== undefined && part !== "")
    .join(" · ");
}

function overviewFacts(video: Video, owner: boolean): Fact[] {
  const f = t.player.facts;
  const text = t.player.tabs;
  // While the probe is pending or failed, File & media says why these are missing.
  const probed = video.probeState === "done";
  const length = probed ? formatDuration(video.durationMs) : "";
  const resolution = probed ? formatResolution(video) : "";
  const facts: (Fact | null)[] = [
    length === "" ? null : { id: "length", term: f.duration, value: length },
    resolution === ""
      ? null
      : { id: "resolution", term: text.resolution, value: resolution },
    {
      id: "added",
      term: f.added,
      value: <DateValue label={f.added} value={video.addedAt} />,
    },
    owner && video.progress !== undefined
      ? {
          id: "last-watched",
          term: text.lastWatched,
          value: <DateValue label={text.lastWatched} value={video.progress.updatedAt} />,
        }
      : null,
  ];
  return facts.filter((fact): fact is Fact => fact !== null);
}

/** The description, the one-line work summary and the four facts; work information is the owner's. */
export default function OverviewPanel({
  video,
  owner,
}: {
  video: Video;
  owner: boolean;
}) {
  const description = owner ? video.workInfo?.description : undefined;
  const summary = owner ? workSummary(video.workInfo) : "";
  const hasMain = description !== undefined || summary !== "";
  return (
    <PanelColumns
      main={
        hasMain ? (
          <div className="flex flex-col gap-4">
            {description !== undefined && (
              // Remount per video so the fold does not carry over.
              <Description key={video.id} text={description} />
            )}
            {summary !== "" && (
              <p className="text-xs text-muted-foreground wrap-anywhere">{summary}</p>
            )}
          </div>
        ) : undefined
      }
      side={<FactList variant="divided" facts={overviewFacts(video, owner)} />}
    />
  );
}
