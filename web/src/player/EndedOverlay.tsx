import { Play, RotateCcw } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import Button from "../ui/Button";
import { VideoThumbnail, videoLinkLabel } from "./RelatedVideos";
import { Dimmed } from "./StatusOverlays";

/**
 * EndedOverlay は最後まで再生したときの層である（要件 14）。
 *
 * 次の動画（同じフォルダの自然順の次）があれば、それと「次を再生」「もう一度見る」を出す。
 * 無ければ「もう一度見る」だけにする。自動では再生せず、カウントダウンも出さない。
 *
 * `takeFocus` が真のとき（フォーカスがプレイヤーの中にあったとき）だけ、主な操作へ
 * フォーカスを移す。プレイヤーの外にいる人のフォーカスは奪わない。
 */
export default function EndedOverlay({
  next,
  backTo,
  takeFocus,
  onReplay,
  onPlayNext,
}: {
  next: Video | undefined;
  backTo: string;
  takeFocus: boolean;
  onReplay: () => void;
  onPlayNext: () => void;
}) {
  const primary = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (takeFocus) primary.current?.focus();
  }, [takeFocus]);

  return (
    <Dimmed>
      <span role="status" className="sr-only">
        再生が終わりました
      </span>
      {next === undefined ? (
        <Button ref={primary} variant="secondary" onClick={onReplay}>
          <RotateCcw aria-hidden="true" />
          もう一度見る
        </Button>
      ) : (
        <div className="pointer-events-auto flex w-full max-w-lg flex-col gap-3 rounded-lg bg-navbar p-5">
          <span className="text-xs font-semibold text-accent">次の動画</span>
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
          <div className="flex flex-wrap gap-2">
            <Button ref={primary} variant="primary" onClick={onPlayNext}>
              <Play aria-hidden="true" />
              次を再生
            </Button>
            <Button variant="secondary" onClick={onReplay}>
              <RotateCcw aria-hidden="true" />
              もう一度見る
            </Button>
          </div>
        </div>
      )}
    </Dimmed>
  );
}
