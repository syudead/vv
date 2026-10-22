import { useEffect, useState } from "react";
import { Link } from "react-router";

import type { Video } from "../../api/client";
import { personFaceSrc } from "../../api/credits";
import {
  listVideoFaceAppearances,
  type VideoFaceAppearance,
  type VideoFaceAppearanceList,
} from "../../api/personFaces";
import type { WorkInfo, WorkPersonRole } from "../../api/workInfo";
import { creditDisplayName } from "../../credits/displayName";
import { errorText, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { formatDuration } from "../../lib/format";
import { CardGrid } from "../../ui/patterns/card-grid";
import { PersonFace } from "../../ui/PersonFace";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { SpanBar } from "../../ui/SpanBar";
import type { PlayerControls } from "../playerControls";
import { PanelSection } from "./panelParts";

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

function personPath(id: number): string {
  return `/people/${String(id)}`;
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
          className="flex min-w-0 flex-col items-center gap-2 rounded-md p-1 text-center transition-shadow hover:shadow-card-hover motion-reduce:transition-none"
        >
          <PersonFace
            src={personFaceSrc({ id: person.id, imageRevision })}
            name={creditDisplayName(person)}
            shape="card"
          />
          <span className="flex w-full min-w-0 flex-col items-center">
            <PersonName person={person} className="w-full" />
            <span className="w-full truncate text-xs text-muted-foreground">
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
  item: VideoFaceAppearance;
  lengthSeconds: number | undefined;
  player: SpanPlayer;
}) {
  const name = creditDisplayName(item.person);
  return (
    <li className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
      <Link
        to={personPath(item.person.id)}
        className="flex min-w-0 items-center gap-2 rounded-md sm:w-chip-label sm:shrink-0"
      >
        <PersonFace
          src={personFaceSrc({ id: item.person.id, imageRevision: item.imageRevision })}
          name={name}
          shape="option"
        />
        <PersonName person={item.person} />
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
    return (
      <p
        role="alert"
        className="flex flex-wrap items-center gap-2 text-sm text-destructive"
      >
        {text.seenFailed(read.reason)}
        <Button variant="link" size="sm" className="h-auto px-0" onClick={retry}>
          {t.common.retry}
        </Button>
      </p>
    );
  }
  const { analysed, items } = read.list;
  if (!analysed || items.length === 0) return null;
  const lengthSeconds =
    video.durationMs === undefined || video.durationMs <= 0
      ? undefined
      : video.durationMs / 1000;
  const pressable = player !== null && lengthSeconds !== undefined;
  return (
    <PanelSection title={text.seenInVideo}>
      {pressable && <p className="text-sm text-muted-foreground">{text.seenHint}</p>}
      <ul className="flex flex-col gap-3">
        {items.map((item) => (
          <SeenRow
            key={item.appearanceId}
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
