import { Play, X } from "lucide-react";
import { type CSSProperties, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { getVideo, isAborted, RequestFailed, type Video } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import Button from "../ui/Button";
import { VideoThumbnail, videoLinkLabel } from "./RelatedVideos";
import { Dimmed } from "./StatusOverlays";

/** 予告から次のメンバーを再生するまでの秒数（specs/017-folder-groups/ui-design.md「Autoplay notice」）。 */
export const autoplayNoticeSeconds = 5;

/** autoplayAnnouncement は予告が出たときに一度だけ読み上げる文である。 */
export function autoplayAnnouncement(title: string): string {
  return `再生が終わりました。${String(autoplayNoticeSeconds)} 秒後に次の動画「${title}」を再生します`;
}

/**
 * AutoplayNotice は、グループのメンバーの再生が終わったときに、次のメンバーを自動で
 * 再生するまでの予告の層である（親 Issue 要件 27、plan の Structural Decisions 12）。
 *
 * - 出ている間だけ数え、0 になったら次のメンバーがまだあるかを `GET /api/videos/{id}` で
 *   確かめてから `onAdvance` を呼ぶ。404 なら `onGone` を呼び、先へ進まない。
 * - 出ている間は、次のメンバーを名指しする `video` イベント（所有者だけ）を見張り、
 *   届いたら同じく確かめる。次のメンバーは開いたときの関連動画の応答で決まり、
 *   ここでは差し替えない。
 * - 始まったことは `role="status"` で一度だけ伝え、残り秒数と帯は読み上げさせない。
 * - `takeFocus` が真のとき（フォーカスがプレイヤーの中にあったとき）だけ「取り消す」へ
 *   フォーカスを移す。
 */
export default function AutoplayNotice({
  next,
  backTo,
  takeFocus,
  watchEvents,
  onCancel,
  onPlayNow,
  onAdvance,
  onGone,
}: {
  next: Video;
  backTo: string;
  takeFocus: boolean;
  /** 変化の知らせを見張るか。`/api/events` は所有者だけのものなので、ゲストでは偽。 */
  watchEvents: boolean;
  onCancel: () => void;
  onPlayNow: () => void;
  onAdvance: () => void;
  onGone: () => void;
}) {
  const cancel = useRef<HTMLButtonElement | null>(null);
  const [remaining, setRemaining] = useState(autoplayNoticeSeconds);
  // 帯は出た直後から縮み始める。最初の描画は満ちた状態で置き、次の描画で縮め始める。
  const [started, setStarted] = useState(false);
  const latest = useRef({ onAdvance, onGone });
  useEffect(() => {
    latest.current = { onAdvance, onGone };
  }, [onAdvance, onGone]);

  useEffect(() => {
    if (takeFocus) cancel.current?.focus();
  }, [takeFocus]);

  useEffect(() => {
    const frame = requestAnimationFrame(() => setStarted(true));
    // 描画の遅れで秒が延びないよう、出たときから1秒ごとに数える。
    const timer = setInterval(() => {
      setRemaining((value) => Math.max(value - 1, 0));
    }, 1000);
    return () => {
      cancelAnimationFrame(frame);
      clearInterval(timer);
    };
  }, []);

  // 確かめは出ている間だけ行い、層が消えたら（取り消し・移動・画面を離れた）打ち切る。
  const checks = useRef(new Set<AbortController>());
  useEffect(() => {
    const running = checks.current;
    return () => {
      for (const controller of running) controller.abort();
      running.clear();
    };
  }, []);

  const nextId = next.id;
  useEffect(() => {
    const confirm = async (): Promise<boolean> => {
      const controller = new AbortController();
      checks.current.add(controller);
      try {
        await getVideo(nextId, controller.signal);
        return true;
      } catch (failure) {
        if (isAborted(failure) || controller.signal.aborted) return false;
        if (failure instanceof RequestFailed && failure.status === 404) {
          latest.current.onGone();
          return false;
        }
        // 一時的な失敗では消えたとは言えない。移った先の画面が状態を伝える。
        return true;
      } finally {
        checks.current.delete(controller);
      }
    };

    if (remaining > 0) return;
    let active = true;
    void confirm().then((present) => {
      if (active && present) latest.current.onAdvance();
    });
    return () => {
      active = false;
    };
  }, [nextId, remaining]);

  useEffect(() => {
    if (!watchEvents) return;
    const confirmGone = () => {
      const controller = new AbortController();
      checks.current.add(controller);
      getVideo(nextId, controller.signal).then(
        () => checks.current.delete(controller),
        (failure: unknown) => {
          checks.current.delete(controller);
          if (controller.signal.aborted || isAborted(failure)) return;
          if (failure instanceof RequestFailed && failure.status === 404) {
            latest.current.onGone();
          }
        },
      );
    };
    return subscribeServerEvents({
      video: (changed) => {
        if (changed === nextId) confirmGone();
      },
      // つなぎ直したときは、切れていた間の知らせを受け取っていない。
      open: (reconnected) => {
        if (reconnected) confirmGone();
      },
    });
  }, [nextId, watchEvents]);

  const fraction = (value: number) => `${String((value / autoplayNoticeSeconds) * 100)}%`;
  // なめらかに縮める帯は、1 秒先の長さへ 1 秒かけて動かす。動きを減らす設定では、
  // 秒数が減るたびに今の長さへ段階で縮める。
  const bar = {
    "--vv-countdown-smooth": fraction(started ? Math.max(remaining - 1, 0) : remaining),
    "--vv-countdown-step": fraction(remaining),
  } as CSSProperties;

  return (
    <Dimmed>
      <span role="status" className="sr-only">
        {autoplayAnnouncement(next.title)}
      </span>
      <div className="pointer-events-auto flex w-full max-w-lg flex-col gap-3 rounded-lg bg-navbar p-5">
        <div className="flex items-baseline justify-between gap-3">
          <span className="text-xs font-semibold text-accent">続けて再生</span>
          <span aria-hidden="true" className="text-xs text-fg-muted tabular-nums">
            {remaining} 秒後
          </span>
        </div>
        <Link
          to={`/videos/${String(next.id)}`}
          state={{ from: backTo }}
          aria-label={videoLinkLabel(next)}
          className="flex items-start gap-3 rounded-md"
        >
          <VideoThumbnail video={next} className="hidden w-56 sm:block" />
          <span className="line-clamp-2 min-w-0 text-base font-semibold text-fg [overflow-wrap:anywhere]">
            {next.title}
          </span>
        </Link>
        <div
          aria-hidden="true"
          data-countdown=""
          className="flex h-1 w-full overflow-hidden rounded-full bg-fg-subtle/50"
          style={bar}
        >
          <div className="h-full w-(--vv-countdown-smooth) rounded-full bg-accent transition-[width] duration-1000 ease-linear motion-reduce:w-(--vv-countdown-step) motion-reduce:transition-none" />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button ref={cancel} variant="secondary" onClick={onCancel}>
            <X aria-hidden="true" />
            取り消す
          </Button>
          <Button variant="primary" onClick={onPlayNow}>
            <Play aria-hidden="true" />
            今すぐ再生
          </Button>
        </div>
      </div>
    </Dimmed>
  );
}
