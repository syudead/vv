import type { Video } from "../../api/client";
import type { WorkInfo } from "../../api/workInfo";
import { creditDisplayName } from "../../credits/displayName";
import { t } from "../../i18n";
import { formatDuration, formatResolution } from "../../lib/format";
import { type Fact, FactList } from "../../ui/patterns/fact-list";
import { Description } from "../WorkInfo";
import { DateValue, PanelColumns, PanelSection } from "./panelParts";

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
    // A virtual video has no File & media tab, so its owner reads the last edit here.
    owner && video.virtual
      ? {
          id: "edited",
          term: f.edited,
          value: <DateValue label={f.edited} value={video.updatedAt} />,
        }
      : null,
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

/**
 * At the left, the owner's description and work summary, each under its own heading; at the right,
 * the facts.
 */
export default function OverviewPanel({
  video,
  owner,
}: {
  video: Video;
  owner: boolean;
}) {
  const description = owner ? video.workInfo?.description : undefined;
  const summary = owner ? workSummary(video.workInfo) : "";
  const text = t.player.tabs;
  const hasMain = description !== undefined || summary !== "";
  return (
    <PanelColumns
      main={
        hasMain ? (
          <>
            {description !== undefined && (
              <PanelSection title={text.description}>
                {/* Remount per video so the fold does not carry over. */}
                <Description key={video.id} text={description} lines="many" />
              </PanelSection>
            )}
            {summary !== "" && (
              <PanelSection title={text.work}>
                <p className="text-xs text-muted-foreground wrap-anywhere">{summary}</p>
              </PanelSection>
            )}
          </>
        ) : undefined
      }
      side={<FactList variant="divided" facts={overviewFacts(video, owner)} />}
    />
  );
}
