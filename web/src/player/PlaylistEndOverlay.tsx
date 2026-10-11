import { RotateCcw } from "lucide-react";
import { useEffect, useRef } from "react";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Dimmed } from "./StatusOverlays";

/**
 * PlaylistEndOverlay はプレイリストの最後の項目を見終えたときの層である
 * （specs/044-playlists/ui-design.md「Video page in a playlist」End of playlist、要件 9）。
 * 何も進めず、カウントダウンも出さない。「もう一度見る」だけを置く。
 *
 * `takeFocus` が真のとき（フォーカスがプレイヤーの中にあったとき）だけ、その操作へ
 * フォーカスを移す。
 */
export default function PlaylistEndOverlay({
  name,
  takeFocus,
  onReplay,
}: {
  name: string;
  takeFocus: boolean;
  onReplay: () => void;
}) {
  const replay = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (takeFocus) replay.current?.focus();
  }, [takeFocus]);

  return (
    <Dimmed>
      <span role="status" className="sr-only">
        {t.player.playlist.endAnnouncement(name)}
      </span>
      <div
        data-playlist-end=""
        className="flex max-w-lg flex-col items-center gap-3 rounded-lg bg-navbar p-5 text-center shadow-elevated"
      >
        <span className="text-xs font-semibold text-primary">
          {t.player.playlist.end}
        </span>
        <p className="line-clamp-2 text-base font-semibold wrap-anywhere">{name}</p>
        <Button ref={replay} variant="secondary" onClick={onReplay}>
          <RotateCcw aria-hidden="true" />
          {t.player.ended.replay}
        </Button>
      </div>
    </Dimmed>
  );
}
