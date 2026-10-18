import {
  AlertCircle,
  ChevronDown,
  Ellipsis,
  Layers,
  LoaderCircle,
  Star,
  Unlink,
} from "lucide-react";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Link } from "react-router";

import {
  isAborted,
  listVideoVersions,
  makeRepresentativeVersion,
  RequestFailed,
  unbundleVideo,
  type Video,
  type VideoVersions,
} from "../api/client";
import { detailMark, type DetailMark } from "../api/useVideoDetail";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { Skeleton } from "../ui/shadcn/skeleton";
import { useToast } from "../ui/Toast";
import { factTrigger } from "./factTrigger";
import {
  VersionDetailsLine,
  versionDetails,
  versionDetailsText,
} from "../versions/VersionDetails";

export interface VersionsNavigation {
  /** Preserve the current return destination. */
  backTo: string;
  /** Continue playback on the destination when playing or ended. */
  autoplay: boolean;
}

type ListState =
  { kind: "loading" } | { kind: "ready"; versions: VideoVersions } | { kind: "failed" };

function failureIsStale(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    (error.status === 404 || error.reason === "not_bundled")
  );
}

/** Reload versions on every open; keep the popover open and report mutation failures inline. */
export default function VersionsFact({
  video,
  count,
  owner,
  navigation,
  onReplace,
  onRefresh,
  onDissolved,
}: {
  video: Video;
  count: number;
  owner: boolean;
  navigation: VersionsNavigation;
  /** Replace local video data after removing the current version. */
  onReplace: (video: Video, mark: DetailMark) => void;
  onRefresh: () => void;
  /** Let the caller choose focus when closing also removes the trigger. */
  onDissolved: () => void;
}) {
  const toast = useToast();
  const headingId = useId();
  const [open, setOpen] = useState(false);
  const [list, setList] = useState<ListState>({ kind: "loading" });
  const [pending, setPending] = useState<ReadonlySet<number>>(() => new Set());
  const [failure, setFailure] = useState<UiText | null>(null);
  const listRef = useRef<HTMLUListElement | null>(null);
  // Use the latest list so consecutive removal responses compose.
  const latestList = useRef(list);
  latestList.current = list;
  const request = useRef<AbortController | null>(null);
  const focusFirst = useRef(false);
  // Do not return focus to a trigger that is about to disappear.
  const dissolving = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      request.current?.abort();
    };
  }, []);

  // Fewer than two versions means membership changed; close and refresh the stale count.
  const onDwindled = useRef<() => void>(() => undefined);

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
          if (versions.items.length < 2) onDwindled.current();
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

  const loadedFor = useRef<Video | null>(null);
  useEffect(() => {
    if (!open) {
      loadedFor.current = null;
      return;
    }
    if (loadedFor.current === video) return;
    const first = loadedFor.current === null;
    loadedFor.current = video;
    if (first) focusFirst.current = true;
    load(first);
  }, [load, open, video]);

  useEffect(() => {
    if (list.kind !== "ready" || !focusFirst.current) return;
    focusFirst.current = false;
    const root = listRef.current;
    const target =
      root?.querySelector<HTMLElement>("a[href]") ?? root?.querySelector("button");
    target?.focus();
  }, [list]);

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setFailure(null);
      request.current?.abort();
    }
  };

  const dissolve = () => {
    dissolving.current = true;
    changeOpen(false);
    onDissolved();
  };

  onDwindled.current = () => {
    dissolve();
    onRefresh();
  };

  const run = (target: Video, action: "representative" | "remove") => {
    if (pending.has(target.id)) return;
    setFailure(null);
    setPending((previous) => new Set(previous).add(target.id));
    const mark = detailMark();
    const index =
      list.kind === "ready"
        ? list.versions.items.findIndex((item) => item.id === target.id)
        : -1;
    const done = () =>
      setPending((previous) => {
        const next = new Set(previous);
        next.delete(target.id);
        return next;
      });
    const onFailure = (error: unknown) => {
      if (!alive.current) return;
      done();
      setFailure(t.player.versions.changeFailed(errorText(error)));
      if (failureIsStale(error)) onRefresh();
    };

    if (action === "representative") {
      makeRepresentativeVersion(target.id).then((versions) => {
        if (!alive.current) return;
        done();
        setList({ kind: "ready", versions });
        onRefresh();
      }, onFailure);
      return;
    }

    unbundleVideo(target.id).then((removed) => {
      if (!alive.current) return;
      done();
      toast(t.player.versions.removed(target.title));
      if (target.id === video.id) {
        dissolve();
        onReplace(removed, mark);
        return;
      }
      const latest = latestList.current;
      const items =
        latest.kind === "ready"
          ? latest.versions.items.filter((item) => item.id !== target.id)
          : [];
      if (latest.kind === "ready") {
        setList({ kind: "ready", versions: { ...latest.versions, items } });
      }
      if (items.length <= 1) {
        dissolve();
        onRefresh();
        return;
      }
      // After removal, focus the replacement row or its predecessor.
      requestAnimationFrame(() => {
        const rows = listRef.current?.querySelectorAll<HTMLElement>(":scope > li");
        if (rows === undefined || rows.length === 0) return;
        const row = rows[Math.min(Math.max(index, 0), rows.length - 1)];
        row?.querySelector<HTMLElement>("a[href], button")?.focus();
      });
    }, onFailure);
  };

  const shown = list.kind === "ready" ? list.versions.items.length : count;

  return (
    <li className="flex items-center whitespace-nowrap">
      <Popover open={open} onOpenChange={changeOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            aria-label={t.player.versions.show(shown)}
            title={t.player.versions.show(shown)}
            className={factTrigger}
          >
            <Layers aria-hidden="true" />
            <span className="tabular-nums">{t.player.versions.count(shown)}</span>
            <ChevronDown aria-hidden="true" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          side="bottom"
          align="start"
          aria-labelledby={headingId}
          className="w-versions-popover gap-0 overflow-hidden p-0"
          onOpenAutoFocus={(event) => {
            event.preventDefault();
          }}
          onCloseAutoFocus={(event) => {
            if (!dissolving.current) return;
            dissolving.current = false;
            event.preventDefault();
          }}
        >
          <h2 id={headingId} className="sr-only">
            {t.player.versions.label}
          </h2>
          {list.kind === "loading" && (
            <div className="flex flex-col gap-1 p-2" aria-busy="true">
              {Array.from({ length: Math.max(count, 1) }, (_, index) => (
                <Skeleton key={index} className="h-10" />
              ))}
            </div>
          )}
          {list.kind === "failed" && (
            <div className="flex items-center gap-2 px-3 py-2">
              <p role="alert" className="text-sm text-destructive">
                {t.player.versions.loadFailed}
              </p>
              <Button variant="ghost" size="sm" onClick={() => load(true)}>
                {t.player.versions.retry}
              </Button>
            </div>
          )}
          {list.kind === "ready" && (
            <ul
              ref={listRef}
              aria-label={t.player.versions.label}
              className="max-h-popover-wide divide-y divide-border overflow-y-auto"
            >
              {list.versions.items.map((item) => (
                <VersionRow
                  key={item.id}
                  item={item}
                  current={item.id === video.id}
                  representative={item.id === list.versions.representativeId}
                  owner={owner}
                  pending={pending.has(item.id)}
                  navigation={navigation}
                  onMakeRepresentative={() => run(item, "representative")}
                  onRemove={() => run(item, "remove")}
                />
              ))}
            </ul>
          )}
          {failure !== null && (
            <p
              role="alert"
              className="flex items-center gap-2 border-t border-border px-3 py-2 text-sm text-destructive"
            >
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              {failure}
            </p>
          )}
        </PopoverContent>
      </Popover>
    </li>
  );
}

function VersionRow({
  item,
  current,
  representative,
  owner,
  pending,
  navigation,
  onMakeRepresentative,
  onRemove,
}: {
  item: Video;
  current: boolean;
  representative: boolean;
  owner: boolean;
  pending: boolean;
  navigation: VersionsNavigation;
  onMakeRepresentative: () => void;
  onRemove: () => void;
}) {
  const details = versionDetails(item);
  // Owners can inspect absolute paths; guests see only relative locations.
  //
  const fullPlace = owner ? (item.location?.path ?? details.place) : details.place;
  const detailsTitle = versionDetailsText(details, " · ", fullPlace);
  const spoken = versionDetailsText(details, " ");

  const text = (
    <>
      <span className="flex min-w-0 items-baseline gap-2">
        {current && <span className="sr-only">{t.player.versions.nowPlaying} </span>}
        <span className="min-w-0 truncate text-sm" title={item.title}>
          {item.title}
        </span>
        {representative && (
          <span className="shrink-0 text-xs text-muted-foreground">
            {t.player.versions.representative}
          </span>
        )}
      </span>
      <VersionDetailsLine details={details} title={detailsTitle} />
    </>
  );

  return (
    <li
      aria-current={current ? "true" : undefined}
      className={cn(
        "flex items-center gap-x-2 pr-2",
        current && "border-l-2 border-primary bg-primary-soft",
      )}
    >
      {current ? (
        <div className="flex min-w-0 flex-1 flex-col gap-0.5 py-2 pl-3">{text}</div>
      ) : (
        <Link
          to={`/videos/${String(item.id)}`}
          state={{ from: navigation.backTo, autoplay: navigation.autoplay }}
          aria-label={t.player.versions.play(item.title, spoken)}
          // Inset the focus ring to prevent clipping at the popover edge.
          className="flex min-w-0 flex-1 flex-col gap-0.5 py-2 pl-3 transition-colors hover:bg-accent focus-visible:-outline-offset-2"
        >
          {text}
        </Link>
      )}
      {owner && (
        <RowActions
          title={item.title}
          representative={representative}
          pending={pending}
          onMakeRepresentative={onMakeRepresentative}
          onRemove={onRemove}
        />
      )}
    </li>
  );
}

function RowActions({
  title,
  representative,
  pending,
  onMakeRepresentative,
  onRemove,
}: {
  title: string;
  representative: boolean;
  pending: boolean;
  onMakeRepresentative: () => void;
  onRemove: () => void;
}) {
  const label = t.player.versions.more(title);
  const [menuOpen, setMenuOpen] = useState(false);
  // Keep the same button while submitting to retain focus without reopening its menu.
  return (
    <DropdownMenu
      open={menuOpen && !pending}
      onOpenChange={(next) => setMenuOpen(next && !pending)}
    >
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          aria-disabled={pending || undefined}
          className="shrink-0 text-muted-foreground aria-disabled:cursor-default"
        >
          {pending ? (
            <LoaderCircle
              aria-hidden="true"
              className="animate-spin motion-reduce:animate-none"
            />
          ) : (
            <Ellipsis aria-hidden="true" />
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {!representative && (
          <>
            <DropdownMenuItem
              onSelect={() => {
                setMenuOpen(false);
                onMakeRepresentative();
              }}
            >
              <Star aria-hidden="true" />
              {t.player.versions.makeRepresentative}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        )}
        <DropdownMenuItem
          onSelect={() => {
            setMenuOpen(false);
            onRemove();
          }}
        >
          <Unlink aria-hidden="true" />
          {t.player.versions.remove}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
