import { LoaderCircle, Star } from "lucide-react";
import { type RefObject, useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  isAborted,
  listVideoVersions,
  makeRepresentativeVersion,
  RequestFailed,
  unbundleVideo,
  type Video,
  type VideoVersions,
} from "../../api/client";
import { type DetailMark, detailMark } from "../../api/useVideoDetail";
import { errorText, t, type UiText } from "../../i18n";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { useToast } from "../../ui/Toast";
import {
  VersionDetailsLine,
  versionDetails,
  versionDetailsText,
} from "../../versions/VersionDetails";
import { VideoThumbnail } from "../RelatedVideos";
import { FailureLine } from "./panelParts";

export interface VersionsNavigation {
  /** Preserve the current return destination. */
  backTo: string;
  /** Continue playback on the destination when playing or ended. */
  autoplay: boolean;
}

export interface VersionsPanelProps {
  video: Video;
  owner: boolean;
  navigation: VersionsNavigation;
  /** Replace the page's video after removing the playing version. */
  onReplace: (video: Video, mark: DetailMark) => void;
  /** Reread the video; its version count or membership changed. */
  onRefresh: () => void;
  /** Move focus out of the panel when the focused row or the whole tab goes away. */
  onLeave: () => void;
}

type ListState =
  { kind: "loading" } | { kind: "ready"; versions: VideoVersions } | { kind: "failed" };

type RowAction = "representative" | "remove";

function failureIsStale(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    (error.status === 404 || error.reason === "not_bundled")
  );
}

/**
 * Read the list when the panel mounts and again, without the skeleton, whenever the video response
 * is replaced. A list of fewer than two means membership changed elsewhere, so the video is reread.
 */
function useVersionsList(video: Video, onDwindled: () => void) {
  const [list, setList] = useState<ListState>({ kind: "loading" });
  const request = useRef<AbortController | null>(null);
  const dwindled = useRef(onDwindled);
  dwindled.current = onDwindled;

  const load = useCallback(
    (showLoading: boolean) => {
      request.current?.abort();
      const controller = new AbortController();
      request.current = controller;
      if (showLoading) setList({ kind: "loading" });
      listVideoVersions(video.id, controller.signal).then(
        (versions) => {
          if (controller.signal.aborted) return;
          setList({ kind: "ready", versions });
          if (versions.items.length < 2) dwindled.current();
        },
        (error: unknown) => {
          if (isAborted(error) || controller.signal.aborted) return;
          setList((previous) =>
            previous.kind === "ready" ? previous : { kind: "failed" },
          );
        },
      );
    },
    [video.id],
  );

  const loaded = useRef(false);
  useEffect(() => {
    load(!loaded.current);
    loaded.current = true;
  }, [load, video]);
  useEffect(() => () => request.current?.abort(), []);

  return { list, setList, retry: () => load(true) };
}

function rowElement(list: HTMLUListElement | null, id: number): HTMLElement | null {
  return (
    list?.querySelector<HTMLElement>(`:scope > li[data-version-id="${String(id)}"]`) ??
    null
  );
}

function rowHoldsFocus(list: HTMLUListElement | null, id: number): boolean {
  return rowElement(list, id)?.contains(document.activeElement) ?? false;
}

/** Focus the row's first remaining action once its focused button has been removed. */
function refocusRow(listRef: RefObject<HTMLUListElement | null>, id: number) {
  requestAnimationFrame(() => {
    const active = document.activeElement;
    if (active !== null && active !== document.body && active.isConnected) return;
    rowElement(listRef.current, id)?.querySelector<HTMLElement>("button")?.focus();
  });
}

/** One request per row at a time; rows answer independently, so removals compose on the latest list. */
function useVersionActions(
  props: VersionsPanelProps,
  versions: ReturnType<typeof useVersionsList>,
  listRef: RefObject<HTMLUListElement | null>,
) {
  const { video, onReplace, onRefresh, onLeave } = props;
  const toast = useToast();
  const [pending, setPending] = useState<ReadonlyMap<number, RowAction>>(() => new Map());
  const [failure, setFailure] = useState<UiText | null>(null);
  const [failedFor, setFailedFor] = useState(video.id);
  if (failedFor !== video.id) {
    setFailedFor(video.id);
    setFailure(null);
  }
  const latest = useRef(versions.list);
  latest.current = versions.list;
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  const settle = (id: number) =>
    setPending((previous) => {
      const next = new Map(previous);
      next.delete(id);
      return next;
    });

  const removedOther = (target: Video, index: number) => {
    const current = latest.current;
    if (current.kind !== "ready") return;
    const items = current.versions.items.filter((item) => item.id !== target.id);
    versions.setList({ kind: "ready", versions: { ...current.versions, items } });
    onRefresh();
    if (items.length <= 1) {
      onLeave();
      return;
    }
    // Focus the row that took the removed row's place, or the one before it.
    requestAnimationFrame(() => {
      const rows = listRef.current?.querySelectorAll<HTMLElement>(":scope > li");
      if (rows === undefined || rows.length === 0) return;
      const row = rows[Math.min(Math.max(index, 0), rows.length - 1)];
      row?.querySelector<HTMLElement>("a[href], button")?.focus();
    });
  };

  const run = (target: Video, action: RowAction) => {
    if (pending.has(target.id)) return;
    setFailure(null);
    setPending((previous) => new Map(previous).set(target.id, action));
    const mark = detailMark();
    const list = latest.current;
    const index =
      list.kind === "ready"
        ? list.versions.items.findIndex((item) => item.id === target.id)
        : -1;
    const onFailure = (error: unknown) => {
      if (!alive.current) return;
      settle(target.id);
      setFailure(t.player.versions.changeFailed(errorText(error)));
      if (failureIsStale(error)) onRefresh();
    };

    if (action === "representative") {
      makeRepresentativeVersion(target.id).then((answer) => {
        if (!alive.current) return;
        const focusedRow = rowHoldsFocus(listRef.current, target.id);
        settle(target.id);
        versions.setList({ kind: "ready", versions: answer });
        // The pressed button leaves the new representative's row; keep focus in that row.
        if (focusedRow) refocusRow(listRef, target.id);
        onRefresh();
      }, onFailure);
      return;
    }

    unbundleVideo(target.id).then((removed) => {
      if (!alive.current) return;
      settle(target.id);
      toast(t.player.versions.removed(target.title));
      if (target.id === video.id) {
        onReplace(removed, mark);
        onLeave();
        return;
      }
      removedOther(target, index);
    }, onFailure);
  };

  return { pending, failure, run };
}

/** The versions of the page's video, read when the tab is selected. */
export default function VersionsPanel(props: VersionsPanelProps) {
  const { video, owner, navigation, onRefresh, onLeave } = props;
  const listRef = useRef<HTMLUListElement | null>(null);
  const versions = useVersionsList(video, () => {
    onRefresh();
    onLeave();
  });
  const { pending, failure, run } = useVersionActions(props, versions, listRef);
  const { list } = versions;
  const text = t.player.versions;

  return (
    <div className="flex flex-col gap-6">
      {list.kind === "loading" && (
        <div className="flex flex-col gap-3" aria-busy="true">
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
        </div>
      )}
      {list.kind === "failed" && (
        <div className="flex items-center gap-2">
          <p role="alert" className="text-sm text-destructive">
            {text.loadFailed}
          </p>
          <Button variant="link" size="sm" onClick={versions.retry}>
            {text.retry}
          </Button>
        </div>
      )}
      {list.kind === "ready" && (
        <ul ref={listRef} aria-label={text.label} className="flex flex-col">
          {list.versions.items.map((item) => (
            <VersionRow
              key={item.id}
              item={item}
              current={item.id === video.id}
              representative={item.id === list.versions.representativeId}
              owner={owner}
              pending={pending.get(item.id)}
              navigation={navigation}
              onAction={(action) => run(item, action)}
            />
          ))}
        </ul>
      )}
      <FailureLine failure={failure} />
    </div>
  );
}

interface RowProps {
  item: Video;
  current: boolean;
  representative: boolean;
  owner: boolean;
  pending: RowAction | undefined;
  navigation: VersionsNavigation;
  onAction: (action: RowAction) => void;
}

function VersionRow(props: RowProps) {
  const { item, current, representative, owner } = props;
  const text = t.player.versions;
  const details = versionDetails(item);
  // Owners read the absolute path; guests only the relative place.
  const path = owner ? (item.location?.path ?? details.place) : details.place;

  return (
    <li
      data-version-id={item.id}
      aria-current={current ? "true" : undefined}
      className="flex flex-col gap-3 border-b border-border py-3 sm:flex-row sm:items-center"
    >
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <VideoThumbnail
          video={item}
          selected={current}
          className="w-list-thumb-cell rounded-md"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="line-clamp-2 min-w-0 text-sm wrap-anywhere text-foreground">
              {item.title}
            </span>
            {representative && (
              <Badge variant="secondary">
                <Star aria-hidden="true" />
                {text.representative}
              </Badge>
            )}
            {current && <Badge variant="soft">{text.nowPlaying}</Badge>}
          </div>
          <VersionDetailsLine
            details={details}
            place={false}
            title={versionDetailsText(details, " · ", path)}
          />
          {path !== "" && (
            // Truncate from the left so the file name stays.
            <span
              dir="rtl"
              title={path}
              className="min-w-0 truncate text-left font-mono text-2xs text-muted-foreground"
            >
              <bdi dir="ltr">{path}</bdi>
            </span>
          )}
        </div>
      </div>
      <RowActions {...props} />
    </li>
  );
}

/** `Play` on every row but the playing one, then the owner's quiet actions. */
function RowActions({
  item,
  current,
  representative,
  owner,
  pending,
  navigation,
  onAction,
}: RowProps) {
  const text = t.player.versions;
  if (current && !owner) return null;
  const action = (kind: RowAction, label: UiText) => (
    <Button
      variant="ghost"
      size="sm"
      aria-disabled={pending !== undefined || undefined}
      aria-busy={pending === kind || undefined}
      // Keep the pressed button mounted so it keeps focus while it spins.
      onClick={() => {
        if (pending === undefined) onAction(kind);
      }}
      className="aria-disabled:cursor-default"
    >
      {pending === kind && (
        <LoaderCircle
          aria-hidden="true"
          className="animate-spin motion-reduce:animate-none"
        />
      )}
      {label}
    </Button>
  );
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-1 sm:justify-end">
      {!current && (
        <Button variant="secondary" size="sm" shape="pill" asChild>
          <Link
            to={`/videos/${String(item.id)}`}
            state={{ from: navigation.backTo, autoplay: navigation.autoplay }}
            aria-label={text.play(
              item.title,
              versionDetailsText(versionDetails(item), " "),
            )}
          >
            {text.playWord}
          </Link>
        </Button>
      )}
      {owner && !representative && action("representative", text.makeRepresentative)}
      {owner && action("remove", text.remove)}
    </div>
  );
}
