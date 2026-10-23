import { useEffect, useState } from "react";
import { Link } from "react-router";

import type { Video } from "../../api/client";
import { personFaceSrc } from "../../api/credits";
import {
  listVideoFaceAppearances,
  type VideoFaceAppearanceList,
} from "../../api/personFaces";
import type { WorkInfo, WorkPersonRole } from "../../api/workInfo";
import { creditDisplayName } from "../../credits/displayName";
import { personPath } from "../../credits/person/personPath";
import { errorText, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { formatDuration } from "../../lib/format";
import { CardGrid } from "../../ui/patterns/card-grid";
import { PersonFace } from "../../ui/PersonFace";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { SpanBar } from "../../ui/SpanBar";
import type { TimeSpan } from "../../ui/spanPlacement";
import type { PlayerControls } from "../playerControls";
import { PanelSection, ReadFailure } from "./panelParts";

/** What a span press needs from the player; null while the player is not shown. */
export type SpanPlayer = Pick<PlayerControls, "seekTo" | "play"> | null;

interface CreditRef {
  id: number;
  name: string;
  provisional: boolean;
}

interface CreditedPerson {
  person: CreditRef;
  imageRevision?: number;
  roles: WorkPersonRole[];
}

/** The credited people once each, in the work's order, with every role they hold. */
function creditedPeople(info: WorkInfo | undefined): CreditedPerson[] {
  const byId = new Map<number, CreditedPerson>();
  for (const entry of info?.people ?? []) {
    const known = byId.get(entry.person.id);
    if (known === undefined) {
      byId.set(entry.person.id, {
        person: entry.person,
        imageRevision: entry.imageRevision,
        roles: [entry.role],
      });
    } else if (!known.roles.includes(entry.role)) {
      known.roles.push(entry.role);
    }
  }
  return [...byId.values()];
}

function PersonName({ person, className }: { person: CreditRef; className?: string }) {
  const name = creditDisplayName(person);
  return (
    <span
      title={name}
      className={cn(
        "truncate text-sm",
        person.provisional ? "text-muted-foreground" : "text-foreground",
        className,
      )}
    >
      {name}
    </span>
  );
}

function Credits({ people }: { people: CreditedPerson[] }) {
  const roles = t.credits.personPage.roles;
  return (
    <CardGrid size="face">
      {people.map(({ person, imageRevision, roles: held }) => (
        <Link
          key={person.id}
          to={personPath(person.id)}
          className="flex min-w-0 flex-col items-start gap-1.5 rounded-md text-left transition-shadow hover:shadow-card-hover motion-reduce:transition-none"
        >
          <PersonFace
            src={personFaceSrc({ id: person.id, imageRevision })}
            name={creditDisplayName(person)}
            shape="scene"
          />
          <span className="flex w-full min-w-0 flex-col items-start gap-0.5">
            <PersonName person={person} className="w-full" />
            <span className="w-full truncate text-2xs text-muted-foreground">
              {held.map((role) => roles[role]).join(" · ")}
            </span>
          </span>
        </Link>
      ))}
    </CardGrid>
  );
}

type AppearancesRead =
  | { kind: "loading" }
  | { kind: "ready"; list: VideoFaceAppearanceList }
  | { kind: "failed"; reason: UiText };

/** Reads the video's face appearances on mount, again when the video response is replaced, and on retry. */
function useFaceAppearances(video: Video) {
  const [read, setRead] = useState<AppearancesRead>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setRead({ kind: "loading" });
    listVideoFaceAppearances(video.id, controller.signal).then(
      (list) => {
        if (!controller.signal.aborted) setRead({ kind: "ready", list });
      },
      (error: unknown) => {
        if (!controller.signal.aborted)
          setRead({ kind: "failed", reason: errorText(error) });
      },
    );
    return () => controller.abort();
  }, [video, attempt]);
  return { read, retry: () => setAttempt((n) => n + 1) };
}

interface SeenPerson {
  person: CreditRef;
  imageRevision?: number;
  spans: TimeSpan[];
}

/** Sorted spans with each overlapping run joined into one. */
function joinSpans(spans: TimeSpan[]): TimeSpan[] {
  const out: TimeSpan[] = [];
  for (const span of [...spans].sort((a, b) => a.start - b.start)) {
    const last = out.at(-1);
    if (last !== undefined && span.start <= last.end) {
      last.end = Math.max(last.end, span.end);
    } else {
      out.push({ start: span.start, end: span.end });
    }
  }
  return out;
}

/** One entry per person, in order of their first appearance, since merged people can own several appearances. */
function seenPeople(items: VideoFaceAppearanceList["items"]): SeenPerson[] {
  const byId = new Map<number, SeenPerson>();
  for (const item of items) {
    const known = byId.get(item.person.id);
    if (known === undefined) {
      byId.set(item.person.id, {
        person: item.person,
        imageRevision: item.imageRevision,
        spans: [...item.spans],
      });
    } else {
      known.spans.push(...item.spans);
    }
  }
  return [...byId.values()].map((seen) => ({ ...seen, spans: joinSpans(seen.spans) }));
}

function spanRange(span: { start: number; end: number }): string {
  return t.player.tabs.spanRange(
    formatDuration(span.start * 1000),
    formatDuration(span.end * 1000),
  );
}

function SeenRow({
  item,
  lengthSeconds,
  player,
}: {
  item: SeenPerson;
  lengthSeconds: number | undefined;
  player: SpanPlayer;
}) {
  const name = creditDisplayName(item.person);
  return (
    <li className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
      <Link
        to={personPath(item.person.id)}
        className="flex min-w-0 items-center gap-2 rounded-md sm:shrink-0"
      >
        <PersonFace
          src={personFaceSrc({ id: item.person.id, imageRevision: item.imageRevision })}
          name={name}
          shape="option"
        />
        <PersonName person={item.person} className="sm:w-list-thumb" />
      </Link>
      {lengthSeconds !== undefined && (
        <SpanBar
          className="sm:flex-1"
          lengthSeconds={lengthSeconds}
          spans={item.spans}
          label={t.player.tabs.spansOf(name)}
          spanLabel={spanRange}
          onPress={
            player === null
              ? undefined
              : (span) => {
                  player.seekTo(span.start);
                  player.play();
                }
          }
        />
      )}
    </li>
  );
}

function SeenInVideo({ video, player }: { video: Video; player: SpanPlayer }) {
  const { read, retry } = useFaceAppearances(video);
  const text = t.player.tabs;
  if (read.kind === "loading") {
    return (
      <div role="status" aria-label={text.seenLoading} className="flex flex-col gap-3">
        <Skeleton className="h-12" />
        <Skeleton className="h-12" />
      </div>
    );
  }
  if (read.kind === "failed") {
    return <ReadFailure text={text.seenFailed(read.reason)} onRetry={retry} />;
  }
  const { analysed, items } = read.list;
  if (!analysed || items.length === 0) return null;
  const lengthSeconds =
    video.durationMs === undefined || video.durationMs <= 0
      ? undefined
      : video.durationMs / 1000;
  const pressable = player !== null && lengthSeconds !== undefined;
  const hint =
    lengthSeconds === undefined ? undefined : (
      <span className="text-2xs text-muted-foreground tabular-nums">
        {text.seenHint(formatDuration(video.durationMs), pressable)}
      </span>
    );
  return (
    <PanelSection title={text.seenInVideo} end={hint}>
      <ul className="flex flex-col gap-3">
        {seenPeople(items).map((item) => (
          <SeenRow
            key={item.person.id}
            item={item}
            lengthSeconds={lengthSeconds}
            player={player}
          />
        ))}
      </ul>
    </PanelSection>
  );
}

/** The credited people, then who face recognition saw on screen and when, pressed to play from there. */
export default function PeoplePanel({
  video,
  player,
}: {
  video: Video;
  player: SpanPlayer;
}) {
  return (
    <div className="flex flex-col gap-6">
      <PanelSection title={t.player.tabs.credits}>
        <Credits people={creditedPeople(video.workInfo)} />
      </PanelSection>
      <SeenInVideo video={video} player={player} />
    </div>
  );
}
