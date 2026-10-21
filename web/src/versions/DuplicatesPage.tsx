import { BadgeCheck, Info, Layers, RotateCw } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  dismissVersionCandidate,
  isAborted,
  listVersionCandidates,
  rejudgeVersionCandidate,
  RequestFailed,
  type VersionCandidate,
  type VersionCandidateJudgement,
  type VersionRepresentativeRecommendation,
  type Video,
} from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, formatTime, t } from "../i18n";
import { inProgress } from "../shell/ScanProvider";
import { audioCodecName } from "../player/MediaInfoGroups";
import { VideoThumbnail, videoLinkLabel } from "../player/RelatedVideos";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { PageHeader } from "../ui/patterns/page-header";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "../ui/shadcn/dialog";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";
import BundleDialog from "./BundleDialog";
import { JudgementEvidence } from "./JudgementEvidence";
import { representativeHints } from "./representativeHints";
import { VersionDetailsLine, versionDetails, versionDetailsText } from "./VersionDetails";

const backTo = "/duplicates";

/** Videos are in ascending ID order. */
interface Pair {
  key: string;
  videos: readonly [Video, Video];
  judgement: VersionCandidateJudgement;
  recommendation: VersionRepresentativeRecommendation;
}

type Page =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "ready"; items: Pair[]; total: number; judging: boolean };

/** Ignore malformed pairs with other than two videos. */
function toPair(candidate: VersionCandidate): Pair | null {
  const [first, second] = candidate.videos;
  if (first === undefined || second === undefined || candidate.videos.length !== 2)
    return null;
  return {
    key: `${String(first.id)}-${String(second.id)}`,
    videos: [first, second],
    judgement: candidate.judgement,
    recommendation: candidate.recommendation,
  };
}

function pairIds(pair: Pair): [number, number] {
  return [pair.videos[0].id, pair.videos[1].id];
}

/**
 * Retain rows during refresh and coalesce overlap into one trailing read. Remove decided pairs
 * immediately and restore focus to the next pair, previous pair, or heading.
 */
export default function DuplicatesPage() {
  const toast = useToast();
  const [page, setPage] = useState<Page>({ kind: "loading" });
  const [dismissing, setDismissing] = useState<ReadonlySet<string>>(new Set());
  const [bundling, setBundling] = useState<Pair | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const differentButtons = useRef(new Map<string, HTMLButtonElement>());
  // Update the latest-list ref synchronously so concurrent decision responses before one render cannot resurrect removed pairs.
  const pageRef = useRef(page);
  const showPage = useCallback((next: Page) => {
    pageRef.current = next;
    setPage(next);
  }, []);

  useEffect(() => {
    const previous = document.title;
    document.title = t.versions.duplicates.documentTitle;
    return () => {
      document.title = previous;
    };
  }, []);

  // Serialize reads; coalesce overlapping refreshes into again.
  const fetching = useRef<AbortController | null>(null);
  const again = useRef(false);
  // Remember decisions during reads so stale snapshots cannot restore pairs.
  const decidedDuringFetch = useRef(new Set<string>());

  const load = useCallback(() => {
    if (fetching.current !== null) {
      again.current = true;
      return;
    }
    const controller = new AbortController();
    fetching.current = controller;
    again.current = false;
    decidedDuringFetch.current = new Set();
    listVersionCandidates(controller.signal).then(
      (result) => {
        if (controller.signal.aborted) return;
        fetching.current = null;
        const decided = decidedDuringFetch.current;
        const pairs = result.items
          .map(toPair)
          .filter((pair): pair is Pair => pair !== null);
        const items = pairs.filter((pair) => !decided.has(pair.key));
        const dropped = pairs.length - items.length;
        let total = Math.max(0, result.total - dropped);
        if (items.length === 0 && total > 0) {
          // Refill when local decisions exhaust the visible subset but total exceeds the 200-pair limit; an empty server response needs no refill.
          if (dropped > 0) again.current = true;
          else total = 0;
        }
        showPage({ kind: "ready", items, total, judging: result.judging });
        if (again.current) load();
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        fetching.current = null;
        // Keep rows on refresh failure; show full-page failure only without visible pairs.
        const current = pageRef.current;
        if (current.kind !== "ready" || current.items.length === 0)
          showPage({ kind: "failed" });
        if (again.current) load();
      },
    );
  }, [showPage]);

  useEffect(() => {
    // Refresh on scan completion only, not per-file events or the initial subscription snapshot.
    let lastScan: { id: number; active: boolean } | undefined;
    // Subscribe before reading, then reread on connection open to cover the server subscription gap. Existing shared connections emit no new open event.
    const unsubscribe = subscribeServerEvents({
      scan: (scan) => {
        const active = inProgress(scan);
        const previous = lastScan;
        lastScan = { id: scan.id, active };
        if (previous === undefined || active) return;
        if (previous.active || previous.id !== scan.id) load();
      },
      candidates: load,
      open: load,
    });
    load();
    return () => {
      unsubscribe();
      fetching.current?.abort();
      fetching.current = null;
    };
  }, [load]);

  function retry() {
    showPage({ kind: "loading" });
    load();
  }

  /**
   * Defer focus until dialog restoration completes; refill when visible pairs are exhausted but
   * total remains beyond the server limit.
   */
  function removePair(key: string) {
    decidedDuringFetch.current.add(key);
    const page = pageRef.current;
    if (page.kind !== "ready") return;
    const index = page.items.findIndex((item) => item.key === key);
    if (index < 0) return;
    const items = page.items.filter((_, position) => position !== index);
    const total = Math.max(0, page.total - 1);
    showPage({ ...page, items, total });
    if (items.length === 0 && total > 0) load();
    const next = items[index] ?? items[index - 1];
    const nextKey = next === undefined ? undefined : next.key;
    setTimeout(() => {
      const target =
        nextKey === undefined ? heading.current : differentButtons.current.get(nextKey);
      target?.focus();
    }, 0);
  }

  /** Disable a pair's buttons while one request for it runs. */
  function sendForPair(pair: Pair, send: () => Promise<void>, sent: () => void) {
    const key = pair.key;
    if (dismissing.has(key)) return;
    setDismissing((current) => new Set(current).add(key));
    const settle = () =>
      setDismissing((current) => {
        const rest = new Set(current);
        rest.delete(key);
        return rest;
      });
    send().then(
      () => {
        settle();
        sent();
      },
      (error: unknown) => {
        settle();
        if (error instanceof RequestFailed && error.status === 404) {
          toast(t.versions.duplicates.gone);
          load();
          return;
        }
        toast(errorText(error));
      },
    );
  }

  function dismiss(pair: Pair) {
    sendForPair(
      pair,
      () => dismissVersionCandidate(pairIds(pair)),
      () => {
        removePair(pair.key);
        toast(t.versions.duplicates.dismissed);
      },
    );
  }

  function rejudge(pair: Pair) {
    sendForPair(
      pair,
      () => rejudgeVersionCandidate(pairIds(pair)),
      () => {
        toast(t.versions.duplicates.rejudged);
        load();
      },
    );
  }

  const refilling = page.kind === "ready" && page.items.length === 0 && page.total > 0;
  const loading = page.kind === "loading" || refilling;
  const count = loading
    ? t.versions.duplicates.loading
    : page.kind === "ready"
      ? t.versions.duplicates.count(page.items.length, page.total)
      : null;
  const empty = page.kind === "ready" && page.items.length === 0 && page.total === 0;

  return (
    <ListPage
      header={
        <PageHeader
          titleRef={heading}
          title={t.versions.duplicates.title}
          count={
            <span role="status" aria-live="polite">
              {empty ? "" : count}
            </span>
          }
        />
      }
    >
      {loading && (
        <div className="flex flex-col gap-3" aria-hidden="true">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-16 w-full" />
          ))}
        </div>
      )}

      {page.kind === "failed" && (
        <ErrorState
          title={<h2>{t.versions.duplicates.loadFailed}</h2>}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      )}

      {empty && (
        <EmptyState
          icon={<Layers aria-hidden="true" />}
          title={<h2>{t.versions.duplicates.empty.title}</h2>}
          description={t.versions.duplicates.empty.description}
        />
      )}

      {page.kind === "ready" && page.items.length > 0 && (
        <ul className="flex flex-col divide-y divide-border rounded-md border border-border bg-card text-card-foreground">
          {page.items.map((pair) => {
            const key = pair.key;
            return (
              <PairRow
                key={key}
                pair={pair}
                judging={page.judging}
                pending={dismissing.has(key)}
                differentRef={(element) => {
                  if (element === null) differentButtons.current.delete(key);
                  else differentButtons.current.set(key, element);
                }}
                onDifferent={() => dismiss(pair)}
                onSame={() => setBundling(pair)}
                onRejudge={() => rejudge(pair)}
              />
            );
          })}
        </ul>
      )}

      {/* Keep an open pair dialog even if refresh removes the pair; Bundle's 404 reports the conflict. */}
      {bundling !== null && (
        <BundleDialog
          videoIds={pairIds(bundling)}
          videos={bundling.videos}
          recommendation={bundling.recommendation}
          onClose={() => setBundling(null)}
          onBundled={() => {
            const key = bundling.key;
            setBundling(null);
            removePair(key);
          }}
        />
      )}
    </ListPage>
  );
}

const rejudgeable: ReadonlySet<VersionCandidateJudgement["state"]> = new Set([
  "undecided",
  "failed",
  "no_connection",
]);

function PairRow({
  pair,
  judging,
  pending,
  differentRef,
  onDifferent,
  onSame,
  onRejudge,
}: {
  pair: Pair;
  judging: boolean;
  pending: boolean;
  differentRef: (element: HTMLButtonElement | null) => void;
  onDifferent: () => void;
  onSame: () => void;
  onRejudge: () => void;
}) {
  const [first, second] = pair.videos;
  const { judgement } = pair;
  // Show the video whose frames are on the left of the evidence images first.
  const shown = judgement.leftVideoId === second.id ? [second, first] : [first, second];
  const framed =
    judgement.evidence?.frames !== undefined && judgement.evidence.frames.length > 0;
  return (
    <li className="flex flex-col gap-3 p-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">{t.versions.duplicates.reason}</p>
        {/* Place decisions before video details in DOM order too. */}
        <div className="ml-auto flex flex-wrap gap-2">
          {judging && rejudgeable.has(judgement.state) && (
            <Button
              variant="ghost"
              size="sm"
              disabled={pending}
              aria-label={t.versions.duplicates.rejudgeFor(first.title, second.title)}
              onClick={onRejudge}
            >
              <RotateCw aria-hidden="true" />
              {t.versions.duplicates.rejudge}
            </Button>
          )}
          <Button
            ref={differentRef}
            variant="outline"
            size="sm"
            disabled={pending}
            aria-label={t.versions.duplicates.differentFor(first.title, second.title)}
            onClick={onDifferent}
          >
            {pending && <Spinner aria-hidden="true" />}
            {t.versions.duplicates.different}
          </Button>
          <Button
            size="sm"
            disabled={pending}
            aria-label={t.versions.duplicates.sameFor(first.title, second.title)}
            onClick={onSame}
          >
            <Layers aria-hidden="true" />
            {t.versions.duplicates.same}
          </Button>
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {shown.map((video, index) => (
          <CandidateVideo
            key={video.id}
            video={video}
            other={shown[1 - index] ?? video}
            recommended={
              pair.recommendation.videoId === video.id
                ? t.versions.duplicates.recommendedReason[pair.recommendation.reason]
                : undefined
            }
            side={
              framed
                ? index === 0
                  ? t.versions.duplicates.left
                  : t.versions.duplicates.right
                : undefined
            }
          />
        ))}
      </div>
      <JudgementPanel
        judgement={judgement}
        judging={judging}
        titles={[first.title, second.title]}
      />
    </li>
  );
}

function judgementState(judgement: VersionCandidateJudgement, judging: boolean): string {
  const state = t.versions.judgement.state;
  switch (judgement.state) {
    case "waiting":
      return judging ? state.waiting : t.versions.judgement.judgingOff;
    case "failed": {
      const failed = state.failed(judgement.error ?? "");
      return judgement.retryAt === undefined
        ? failed
        : `${failed} · ${t.versions.judgement.retryAt(formatTime(judgement.retryAt))}`;
    }
    case "undecided":
      return undecidedReason(judgement);
    default:
      return state[judgement.state];
  }
}

/** Name what left the pair undecided, so the headline says more than that it was. */
function undecidedReason(judgement: VersionCandidateJudgement): string {
  const state = t.versions.judgement.state;
  if (judgement.evidence?.conflict === true) return state.undecidedConflict;
  if (judgement.evidence?.byProperties === true) return state.undecidedProperties;
  if (judgement.evidence?.rule === "incomparable") return state.undecidedIncomparable;
  if (judgement.probability !== undefined)
    return state.undecidedAi(t.activity.percent(judgement.probability));
  return state.undecided;
}

/** The automatic judgement's state in one line; the reasons and frames open in a dialog. */
function JudgementPanel({
  judgement,
  judging,
  titles,
}: {
  judgement: VersionCandidateJudgement;
  judging: boolean;
  titles: readonly [string, string];
}) {
  const [open, setOpen] = useState(false);
  const j = t.versions.judgement;
  const headline = judgementState(judgement, judging);
  const evidence = rejudgeable.has(judgement.state) ? judgement.evidence : undefined;
  return (
    <section
      aria-label={j.heading}
      className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-muted px-3 py-2"
    >
      <p className="min-w-0 text-sm text-foreground">{headline}</p>
      {evidence !== undefined && (
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button variant="ghost" size="sm" aria-label={j.whyFor(...titles)}>
              <Info aria-hidden="true" />
              {j.why}
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{j.whyTitle}</DialogTitle>
              <DialogDescription>{headline}</DialogDescription>
            </DialogHeader>
            <JudgementEvidence
              evidence={evidence}
              probability={judgement.probability}
              activityId={judgement.activityId}
              columns={1}
            />
            <DialogFooter>
              <DialogClose asChild>
                <Button variant="outline">{t.common.close}</Button>
              </DialogClose>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}

function fileName(video: Video): string {
  const path = video.location?.path ?? "";
  return path.slice(Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\")) + 1);
}

/** Frame rate, bit rate and audio, which with the thumbnail's length tell two encodes of one video apart. */
function playbackFacts(video: Video): string[] {
  const m = t.player.mediaInfo;
  const media = video.mediaInfo?.video;
  const [track, ...more] = video.mediaInfo?.audioTracks ?? [];
  let audio: string = t.versions.duplicates.noAudio;
  if (track !== undefined) {
    // A space, not the line's separator, keeps one track's codec and channels together.
    audio = [
      track.codec === undefined ? undefined : audioCodecName(track.codec),
      track.channels === undefined ? undefined : m.channels(track.channels),
      more.length === 0 ? undefined : t.versions.duplicates.moreAudio(more.length),
    ]
      .filter((part): part is string => part !== undefined)
      .join(" ");
  } else if (video.audioCodec !== undefined && video.audioCodec !== "")
    audio = audioCodecName(video.audioCodec);
  return [
    media?.frameRate === undefined ? undefined : m.frameRate(media.frameRate),
    media?.bitRate === undefined ? undefined : m.bitRate(media.bitRate),
    video.probeState === "done" ? audio : undefined,
  ].filter((value): value is string => value !== undefined && value !== "");
}

/** Give both videos equal visual emphasis; only the badge and its reason mark the recommended one. */
function CandidateVideo({
  video,
  other,
  recommended,
  side,
}: {
  video: Video;
  other: Video;
  /** The reason it is recommended as the representative. */
  recommended: string | undefined;
  side: string | undefined;
}) {
  const details = versionDetails(video);
  const hints = representativeHints(video, other);
  const specs = { specs: details.specs, place: "" };
  const playback = { specs: playbackFacts(video), place: "" };
  const absolute = video.location?.path ?? details.place;
  const name = fileName(video);
  return (
    <Link
      to={`/videos/${String(video.id)}`}
      state={{ from: backTo }}
      aria-label={videoLinkLabel(video)}
      className="-m-1.5 flex min-w-0 gap-3 rounded-md p-1.5 transition-colors hover:bg-accent"
    >
      <VideoThumbnail video={video} className="w-list-thumb-cell" />
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        {side !== undefined && (
          <span className="text-xs font-medium text-muted-foreground">{side}</span>
        )}
        <span
          className="line-clamp-2 text-sm font-medium wrap-anywhere text-foreground"
          title={video.title}
        >
          {video.title}
        </span>
        {name !== "" && name !== video.title && (
          <span className="truncate text-xs text-muted-foreground" title={name}>
            {name}
          </span>
        )}
        <VersionDetailsLine
          details={playback}
          title={versionDetailsText(playback, " · ")}
        />
        <VersionDetailsLine details={specs} title={versionDetailsText(specs, " · ")} />
        {details.place !== "" && (
          // Preserve the path suffix nearest the file.
          <span
            dir="rtl"
            className="truncate text-left text-xs text-muted-foreground"
            title={absolute}
          >
            <bdi dir="ltr">{details.place}</bdi>
          </span>
        )}
        {hints.length > 0 && (
          <span className="text-xs text-muted-foreground">{hints.join(" · ")}</span>
        )}
        {recommended !== undefined && (
          <span className="flex flex-col gap-0.5">
            <Badge variant="success">
              <BadgeCheck aria-hidden="true" />
              {t.versions.duplicates.recommended}
            </Badge>
            <span className="text-xs text-muted-foreground">{recommended}</span>
          </span>
        )}
      </span>
    </Link>
  );
}
