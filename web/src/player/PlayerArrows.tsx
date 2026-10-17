import { t } from "../i18n";
import NeighborArrows, { type Neighbor } from "./NeighborArrows";
import type { VideoPlaylist } from "./useVideoPlaylist";

/**
 * PlayerArrows はプレイヤーの左右の端の前後のつまみである。プレイリストの文脈を読めたら再生の
 * 応答のステップへ移り、プレイヤーの無い状態の層の上にも描く。終わらない動画から先へ進む道
 * だからである。文脈の外では、プレイヤーが出ている間だけ
 * 関連動画の前後へ移る。文脈を読んでいる間と失敗した間は出さない。
 */
export default function PlayerArrows({
  playlist,
  previous,
  next,
  showPlayer,
  chromeVisible,
  ended,
  container,
}: {
  playlist: VideoPlaylist;
  previous: Neighbor | undefined;
  next: Neighbor | undefined;
  showPlayer: boolean;
  /** 操作バーが見えている。 */
  chromeVisible: boolean;
  ended: boolean;
  container: HTMLElement | null;
}) {
  if (playlist.context.status === "ready")
    return (
      <NeighborArrows
        previous={playlist.previousArrow}
        next={playlist.nextArrow}
        visible={!showPlayer || chromeVisible || ended}
        container={container}
        labels={{
          previous: t.player.playlist.previous,
          next: t.player.playlist.next,
        }}
      />
    );
  if (!showPlayer || playlist.inPlaylist) return null;
  return (
    <NeighborArrows
      previous={previous}
      next={next}
      visible={chromeVisible || ended}
      container={container}
    />
  );
}
