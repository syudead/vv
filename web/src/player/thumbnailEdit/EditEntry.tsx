import { Film, LoaderCircle, WandSparkles } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  discardThumbnailEdit,
  getThumbnailEditReadiness,
  isAborted,
  RequestFailed,
  startThumbnailEdit,
  type ThumbnailEdit,
  type ThumbnailEditStart,
  type Video,
} from "../../api/client";
import type { DetailMark } from "../../api/useVideoDetail";
import { errorText, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { formatDuration } from "../../lib/format";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
} from "../../ui/shadcn/popover";
import ActionButton from "../ActionButton";
import type { ThumbnailCapture } from "../VideoFacts";
import ThumbnailEditor from "./ThumbnailEditor";

export const COMFYUI_SETTINGS = "/settings/connections#comfyui";

/** Treat unknown readiness as unavailable; only the latest settings request may update it. */
function useReadiness(): {
  ready: boolean | null;
  reread: () => Promise<boolean>;
} {
  const [ready, setReady] = useState<boolean | null>(null);
  const latest = useRef<AbortController | null>(null);

  const read = useCallback(async () => {
    latest.current?.abort();
    const controller = new AbortController();
    latest.current = controller;
    const readiness = await getThumbnailEditReadiness(controller.signal);
    if (controller.signal.aborted) throw new DOMException("superseded", "AbortError");
    setReady(readiness.connection);
    return readiness.connection;
  }, []);

  useEffect(() => {
    read().catch(() => undefined);
    return () => latest.current?.abort();
  }, [read]);
  return { ready, reread: read };
}

function framePosition(capture: ThumbnailCapture | undefined): number | null {
  if (capture === undefined || !capture.enabled) return null;
  const raw = capture.positionMs();
  if (raw === null || !Number.isFinite(raw)) return null;
  return Math.max(0, Math.round(raw));
}

/** Capture frame position when opening the source menu and pause before editing. Unavailable connections recheck settings before offering setup guidance. */
export default function EditEntry({
  videoId,
  capture,
  thumbnailUrl,
  report,
  onApplied,
  onStale,
  onEditingChange,
}: {
  videoId: number;
  capture?: ThumbnailCapture;
  thumbnailUrl: string | undefined;
  /** Clear the shared error row with null before starting. */
  report: (failure: UiText | null) => void;
  onApplied: (video: Video, mark: DetailMark) => void;
  onStale?: () => void;
  /** Suspend video-page shortcuts while the editor is open. */
  onEditingChange?: (open: boolean) => void;
}) {
  const text = t.player.thumbnailEdit;
  const { ready, reread } = useReadiness();
  const [pending, setPending] = useState(false);
  const busy = useRef(false);
  const [hintOpen, setHintOpen] = useState(false);
  // Freeze the selected frame position when the menu opens; null disables frame selection.
  const [menu, setMenu] = useState<{ positionMs: number | null } | null>(null);
  const [edit, setEdit] = useState<ThumbnailEdit | null>(null);
  const mounted = useRef(true);
  const editingChange = useRef(onEditingChange);
  editingChange.current = onEditingChange;

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const open = edit !== null;
  const button = useRef<HTMLButtonElement | null>(null);
  const wasOpen = useRef(false);
  useEffect(() => {
    editingChange.current?.(open);
    if (wasOpen.current && !open) button.current?.focus();
    wasOpen.current = open;
  }, [open]);
  useEffect(() => () => editingChange.current?.(false), []);

  const openMenu = () => setMenu({ positionMs: framePosition(capture) });

  const press = async () => {
    if (busy.current) return;
    if (ready === true) {
      openMenu();
      return;
    }
    busy.current = true;
    setPending(true);
    report(null);
    try {
      if (await reread()) {
        if (mounted.current) openMenu();
      } else if (mounted.current) setHintOpen(true);
    } catch (error) {
      if (isAborted(error) || !mounted.current) return;
      report(text.startFailed(errorText(error)));
    } finally {
      busy.current = false;
      if (mounted.current) setPending(false);
    }
  };

  const begin = async (start: ThumbnailEditStart) => {
    setMenu(null);
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    report(null);
    try {
      capture?.pause();
      const session = await startThumbnailEdit(videoId, start);
      if (mounted.current) setEdit(session);
      // Discard newly started sessions immediately if navigation occurred while starting.
      else void discardThumbnailEdit(session.id).catch(() => undefined);
    } catch (error) {
      if (isAborted(error) || !mounted.current) return;
      report(text.startFailed(errorText(error)));
      if (error instanceof RequestFailed && error.status === 404) onStale?.();
    } finally {
      busy.current = false;
      if (mounted.current) setPending(false);
    }
  };

  const framePositionMs = menu?.positionMs ?? null;

  return (
    <>
      <Popover open={hintOpen} onOpenChange={setHintOpen}>
        <PopoverAnchor asChild>
          <span className="inline-flex">
            <DropdownMenu
              open={menu !== null && !pending}
              onOpenChange={(next) => {
                if (next) void press();
                else setMenu(null);
              }}
            >
              <DropdownMenuTrigger asChild>
                <ActionButton
                  ref={button}
                  label={text.entry}
                  aria-disabled={pending || ready !== true || undefined}
                  aria-busy={pending || undefined}
                  className={cn(
                    "aria-disabled:cursor-default data-[state=open]:bg-accent",
                    ready !== true && !pending && "opacity-50",
                  )}
                >
                  {pending ? (
                    <LoaderCircle
                      aria-hidden="true"
                      className="animate-spin motion-reduce:animate-none"
                    />
                  ) : (
                    <WandSparkles aria-hidden="true" />
                  )}
                </ActionButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem
                  disabled={framePositionMs === null}
                  onSelect={() => {
                    if (framePositionMs !== null)
                      void begin({ source: "frame", positionMs: framePositionMs });
                  }}
                >
                  <Film aria-hidden="true" />
                  {framePositionMs === null ? (
                    text.frameOnScreen
                  ) : (
                    <span className="tabular-nums">
                      {text.frameAt(formatDuration(framePositionMs))}
                    </span>
                  )}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => void begin({ source: "thumbnail" })}>
                  <ThumbnailImage url={thumbnailUrl} />
                  {text.currentThumbnail}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </span>
        </PopoverAnchor>
        <PopoverContent side="bottom" align="end">
          <PopoverHeader>
            <PopoverTitle>{text.setupTitle}</PopoverTitle>
            <PopoverDescription>{text.setupBody}</PopoverDescription>
          </PopoverHeader>
          <Button variant="link" size="sm" className="self-start px-0" asChild>
            <Link to={COMFYUI_SETTINGS}>{text.openSettings}</Link>
          </Button>
        </PopoverContent>
      </Popover>
      {edit !== null && (
        <ThumbnailEditor
          key={edit.id}
          edit={edit}
          onApplied={onApplied}
          onClosed={() => setEdit(null)}
        />
      )}
    </>
  );
}

/** Use empty alt because the menu item supplies the name; preserve an empty box while loading. */
function ThumbnailImage({ url }: { url: string | undefined }) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  return (
    <span className="aspect-video h-6 shrink-0 overflow-hidden rounded-sm bg-card">
      {url !== undefined && url !== brokenUrl && (
        <img
          src={url}
          alt=""
          onError={() => setBrokenUrl(url)}
          className="size-full object-cover"
        />
      )}
    </span>
  );
}
