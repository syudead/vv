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

/** COMFYUI_SETTINGS は Settings の Connections のページの ComfyUI の節である。 */
export const COMFYUI_SETTINGS = "/settings/connections#comfyui";

/**
 * useReadiness は編集の入口を有効にできるか（ComfyUI の接続先が保存されているか）を読む。
 * 読めない間・読めなかったときは `null` で、入口は準備ができていないときと同じに見せる。
 * 読み直すときは読みかけを捨て、最後に送った読み取りの答えだけを反映する。
 */
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

/**
 * framePosition は、キャプチャのボタンと同じ条件で画面にコマがあるときのプレイヤーの位置
 * （ミリ秒、整数）を返し、無ければ null を返す。
 */
function framePosition(capture: ThumbnailCapture | undefined): number | null {
  if (capture === undefined || !capture.enabled) return null;
  const raw = capture.positionMs();
  if (raw === null || !Number.isFinite(raw)) return null;
  return Math.max(0, Math.round(raw));
}

/**
 * EditEntry は情報の行の右端の「Edit thumbnail with ComfyUI」と、それが開く元の選択のメニューと
 * 編集画面である（specs/045-comfyui-thumbnails/ui-design.md「Video page: edit entry」、
 * specs/046-thumbnail-image-upload/ui-design.md「Video page: edit entry with the source
 * choice」）。所有者に置き、プレイヤーが出ていない動画にも置く。
 *
 * - 準備ができていれば、押すと元の選択のメニューを開く。コマの項目はメニューを開いた時点の
 *   プレイヤーの位置を名指して送り、画面にコマが無いとき（`capture` が無いか押せない）は
 *   押せない。今のサムネイルの項目はいつも押せる。どちらも再生を止め、編集を始め、画像を
 *   取り出す間は入口が回る。始められなければ情報の行の失敗の 1 行に理由を出す。
 * - 準備ができていなければ薄く出し（フォーカスは受ける）、押すと読み直し、まだなら Settings
 *   への案内の吹き出しを開く。読み直して準備ができていればメニューを開く。
 */
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
  /** プレイヤーが出ているときだけ渡す。 */
  capture?: ThumbnailCapture;
  /** 今のサムネイルの画像。メニューの項目に小さく出す。 */
  thumbnailUrl: string | undefined;
  /** 情報の行の失敗の 1 行。始めるときに null を渡す。 */
  report: (failure: UiText | null) => void;
  onApplied: (video: Video, mark: DetailMark) => void;
  onStale?: () => void;
  /** 編集画面を開いた・閉じた。開いている間、動画ページのキーを止める。 */
  onEditingChange?: (open: boolean) => void;
}) {
  const text = t.player.thumbnailEdit;
  const { ready, reread } = useReadiness();
  const [pending, setPending] = useState(false);
  const busy = useRef(false);
  const [hintOpen, setHintOpen] = useState(false);
  // メニューを開いた時点のコマの位置。null ならコマの項目は押せない。
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
    // 編集画面を閉じたら、フォーカスを入口へ戻す（ui-design「Closing」）。
    if (wasOpen.current && !open) button.current?.focus();
    wasOpen.current = open;
  }, [open]);
  useEffect(() => () => editingChange.current?.(false), []);

  const openMenu = () => setMenu({ positionMs: framePosition(capture) });

  /** press は入口が押された。準備ができていればメニューを開き、無ければ読み直す。 */
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

  /** begin は選んだ元で再生を止めて編集を始める。 */
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
      // 始める間にページを離れたら、始めたセッションをすぐ捨てる。
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

/**
 * ThumbnailImage はメニューの今のサムネイルの項目の小さな画像で、情報の行のサムネイルの項目と
 * 同じ形にする。項目の文字が名前を持つので `alt` は空にする。読めない間は空の箱にする。
 */
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
