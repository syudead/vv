import { Layers } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  dismissVersionCandidate,
  isAborted,
  listVersionCandidates,
  RequestFailed,
  type VersionCandidate,
  type Video,
} from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, t } from "../i18n";
import { inProgress } from "../shell/ScanProvider";
import { VideoThumbnail, videoLinkLabel } from "../player/RelatedVideos";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";
import BundleDialog from "./BundleDialog";
import { VersionDetailsLine, versionDetails, versionDetailsText } from "./VersionDetails";

const backTo = "/duplicates";

/** Videos are in ascending ID order. */
interface Pair {
  key: string;
  videos: readonly [Video, Video];
}

type Page =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "ready"; items: Pair[]; total: number };

/** Ignore malformed pairs with other than two videos. */
function toPair(candidate: VersionCandidate): Pair | null {
  const [first, second] = candidate.videos;
  if (first === undefined || second === undefined || candidate.videos.length !== 2)
    return null;
  return { key: `${String(first.id)}-${String(second.id)}`, videos: [first, second] };
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
        showPage({ kind: "ready", items, total });
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
    showPage({ kind: "ready", items, total });
    if (items.length === 0 && total > 0) load();
    const next = items[index] ?? items[index - 1];
    const nextKey = next === undefined ? undefined : next.key;
    setTimeout(() => {
      const target =
        nextKey === undefined ? heading.current : differentButtons.current.get(nextKey);
      target?.focus();
    }, 0);
  }

  function dismiss(pair: Pair) {
    const key = pair.key;
    if (dismissing.has(key)) return;
    setDismissing((current) => new Set(current).add(key));
    const settle = () =>
      setDismissing((current) => {
        const rest = new Set(current);
        rest.delete(key);
        return rest;
      });
    dismissVersionCandidate(pairIds(pair)).then(
      () => {
        settle();
        removePair(key);
        toast(t.versions.duplicates.dismissed);
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
                pending={dismissing.has(key)}
                differentRef={(element) => {
                  if (element === null) differentButtons.current.delete(key);
                  else differentButtons.current.set(key, element);
                }}
                onDifferent={() => dismiss(pair)}
                onSame={() => setBundling(pair)}
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

function PairRow({
  pair,
  pending,
  differentRef,
  onDifferent,
  onSame,
}: {
  pair: Pair;
  pending: boolean;
  differentRef: (element: HTMLButtonElement | null) => void;
  onDifferent: () => void;
  onSame: () => void;
}) {
  const [first, second] = pair.videos;
  return (
    <li className="flex flex-col gap-3 p-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">{t.versions.duplicates.reason}</p>
        {/* Place decisions before video details in DOM order too. */}
        <div className="ml-auto flex gap-2">
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
        {pair.videos.map((video) => (
          <CandidateVideo key={video.id} video={video} />
        ))}
      </div>
    </li>
  );
}

/** Give both videos equal visual emphasis. */
function CandidateVideo({ video }: { video: Video }) {
  const details = versionDetails(video);
  const specs = { specs: details.specs, place: "" };
  const absolute = video.location?.path ?? details.place;
  return (
    <Link
      to={`/videos/${String(video.id)}`}
      state={{ from: backTo }}
      aria-label={videoLinkLabel(video)}
      className="-m-1.5 flex min-w-0 gap-3 rounded-md p-1.5 transition-colors hover:bg-accent"
    >
      <VideoThumbnail video={video} className="w-list-thumb-cell" />
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <span
          className="line-clamp-2 text-sm font-medium wrap-anywhere text-foreground"
          title={video.title}
        >
          {video.title}
        </span>
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
      </span>
    </Link>
  );
}
