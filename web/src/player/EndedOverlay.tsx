import { Play, RotateCcw } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { VideoThumbnail, videoLinkLabel } from "./RelatedVideos";
import { Dimmed } from "./StatusOverlays";

/**
 * EndedOverlay は最後まで再生したときの層である（要件 14）。
 *
 * 次の動画（同じフォルダの自然順の次。グループのメンバーならグループの中の次）が
 * あれば、それと「次を再生」「もう一度見る」を出す。無ければ「もう一度見る」だけにする。
 * 自動では再生せず、カウントダウンも出さない（メンバーの予告は AutoplayNotice が出し、
 * これは予告を取り消した後の層になる）。
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
        {t.player.ended.announcement}
      </span>
      {next === undefined ? (
        <div className="flex flex-col items-center gap-3 rounded-lg bg-popover p-6 text-popover-foreground text-center shadow-elevated">
          <h2 className="text-lg font-semibold">{t.player.ended.announcement}</h2>
          <Button ref={primary} onClick={onReplay}>
            <RotateCcw aria-hidden="true" />
            {t.player.ended.replay}
          </Button>
        </div>
      ) : (
        <div className="pointer-events-auto flex w-full max-w-lg flex-col gap-3 rounded-lg bg-popover p-6 text-popover-foreground shadow-elevated">
          <h2 className="text-lg font-semibold">{t.player.ended.announcement}</h2>
          <span className="text-sm font-semibold text-primary">
            {t.player.ended.next}
          </span>
          <Link
            to={`/videos/${String(next.id)}`}
            state={{ from: backTo }}
            aria-label={videoLinkLabel(next)}
            className="flex items-start gap-3 rounded-md"
          >
            <VideoThumbnail video={next} className="hidden w-card-0 shrink-0 sm:block" />
            <span className="line-clamp-2 min-w-0 text-base font-semibold wrap-anywhere">
              {next.title}
            </span>
          </Link>
          <div className="flex flex-wrap gap-2">
            <Button ref={primary} onClick={onPlayNext}>
              <Play aria-hidden="true" />
              {t.player.ended.playNext}
            </Button>
            <Button variant="outline" onClick={onReplay}>
              <RotateCcw aria-hidden="true" />
              {t.player.ended.replay}
            </Button>
          </div>
        </div>
      )}
    </Dimmed>
  );
}
