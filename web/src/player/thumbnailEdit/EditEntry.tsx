import { LoaderCircle, WandSparkles } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  discardThumbnailEdit,
  getThumbnailEditReadiness,
  isAborted,
  RequestFailed,
  startThumbnailEdit,
  type ThumbnailEdit,
  type Video,
} from "../../api/client";
import type { DetailMark } from "../../api/useVideoDetail";
import { errorText, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { Button } from "../../ui/shadcn/button";
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

/** COMFYUI_SETTINGS は Settings の ComfyUI の節である。 */
export const COMFYUI_SETTINGS = "/settings#comfyui";

/**
 * useReadiness は編集の入口を有効にできるか（ComfyUI の接続先が保存されているか）を読む。
 * 読めない間・読めなかったときは `null` で、入口は準備ができていないときと同じに見せる。
 */
function useReadiness(): {
  ready: boolean | null;
  reread: () => Promise<boolean>;
} {
  const [ready, setReady] = useState<boolean | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    getThumbnailEditReadiness(controller.signal).then(
      (readiness) => {
        if (!controller.signal.aborted) setReady(readiness.connection);
      },
      () => undefined,
    );
    return () => controller.abort();
  }, []);
  const reread = async () => {
    const readiness = await getThumbnailEditReadiness();
    setReady(readiness.connection);
    return readiness.connection;
  };
  return { ready, reread };
}

/**
 * EditEntry は情報の行の右端の「Edit thumbnail with ComfyUI」と、それが開く編集画面である
 * （specs/045-comfyui-thumbnails/ui-design.md「Video page: edit entry」）。所有者で
 * プレイヤーが出ているときだけ置く。
 *
 * - 準備ができていれば、押すと再生を止め、プレイヤーの位置で編集を始め、コマを取り出す間は
 *   回る。始められなければ情報の行の失敗の 1 行に理由を出す。
 * - 準備ができていなければ薄く出し（フォーカスは受ける）、押すと読み直し、まだなら Settings
 *   への案内の吹き出しを開く。
 * - 始められるコマが無い（キャプチャのボタンと同じ条件）ときは押せない。
 */
export default function EditEntry({
  videoId,
  capture,
  report,
  onApplied,
  onStale,
  onEditingChange,
}: {
  videoId: number;
  capture: ThumbnailCapture;
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
  const [edit, setEdit] = useState<{ session: ThumbnailEdit; positionMs: number } | null>(
    null,
  );
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

  const unavailable = !capture.enabled && !pending;

  const press = async () => {
    if (busy.current || !capture.enabled) return;
    busy.current = true;
    setPending(true);
    report(null);
    try {
      if (ready !== true && !(await reread())) {
        if (mounted.current) setHintOpen(true);
        return;
      }
      const raw = capture.positionMs();
      if (raw === null || !Number.isFinite(raw)) return;
      const positionMs = Math.max(0, Math.round(raw));
      capture.pause();
      const session = await startThumbnailEdit(videoId, positionMs);
      if (mounted.current) setEdit({ session, positionMs });
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

  return (
    <>
      <Popover open={hintOpen} onOpenChange={setHintOpen}>
        <PopoverAnchor asChild>
          <span className="inline-flex">
            <ActionButton
              ref={button}
              label={text.entry}
              aria-disabled={unavailable || pending || ready !== true || undefined}
              aria-busy={pending || undefined}
              onClick={() => void press()}
              className={cn(
                "aria-disabled:cursor-default",
                (unavailable || (ready !== true && !pending)) && "opacity-50",
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
          key={edit.session.id}
          edit={edit.session}
          positionMs={edit.positionMs}
          onApplied={onApplied}
          onClosed={() => setEdit(null)}
        />
      )}
    </>
  );
}
