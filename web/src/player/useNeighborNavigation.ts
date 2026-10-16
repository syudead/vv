import { useNavigate } from "react-router";

import type { RelatedState } from "../api/useVideoDetail";
import type { Neighbor } from "./NeighborArrows";
import type { VideoLinkState } from "./pageDecisions";
import { type PreviousVideo, usePreviousVideo } from "./usePreviousVideo";

/**
 * useNeighborNavigation は、プレイリストの文脈の外の前後の動画と「次を再生」である。
 *
 * グループのメンバーなら、前後はグループの中の並びで、題名はメンバーの並びから引く
 * （関連動画の items は同じグループのメンバーを含まない）。グループに属さない動画の次は
 * 関連動画の並びの先頭で、前は直前に見ていた動画である（issue 931）。左右の端の前後の動画へ
 * 再生中（または見終えた後）に移ったときは、移った先でも再生を続ける（`keepPlaying`）。
 */
export function useNeighborNavigation({
  related,
  trail,
  linkState,
  backTo,
  videoTo,
  keepPlaying,
}: {
  related: RelatedState;
  trail: readonly number[];
  linkState: VideoLinkState;
  backTo: string;
  videoTo: (videoId: number) => string;
  keepPlaying: boolean;
}) {
  const navigate = useNavigate();
  const neighbors = related.kind === "ready" ? related.related : undefined;
  const group = neighbors?.group;
  const findVideo = (targetId: number) =>
    (group?.items ?? neighbors?.items)?.find((item) => item.id === targetId);
  const nextVideo =
    neighbors?.nextId !== undefined ? findVideo(neighbors.nextId) : undefined;
  // 直前に見ていた動画は、まだ見られると確かめられたときだけ ‹ にする。
  const previousVideo = usePreviousVideo(trail.at(-1));

  const playNext = () => {
    if (nextVideo === undefined) return;
    void navigate(videoTo(nextVideo.id), {
      state: { ...linkState, autoplay: true },
    });
  };
  const neighbor = (targetId: number | undefined): Neighbor | undefined =>
    targetId === undefined
      ? undefined
      : {
          title: findVideo(targetId)?.title,
          go: () =>
            void navigate(videoTo(targetId), {
              state: { ...linkState, autoplay: keepPlaying },
            }),
        };
  // ‹ で直前の動画へ戻るときは、道筋からその動画を外す。もう一度 ‹ を押すとさらに前へ戻る。
  const backNeighbor = (video: PreviousVideo): Neighbor => ({
    title: video.title,
    go: () =>
      void navigate(`/videos/${String(video.id)}`, {
        state: { from: backTo, trail: trail.slice(0, -1), autoplay: keepPlaying },
      }),
  });
  let previous: Neighbor | undefined;
  if (neighbors === undefined) previous = undefined;
  else if (group !== undefined) previous = neighbor(neighbors.prevId);
  else if (previousVideo !== undefined) previous = backNeighbor(previousVideo);

  return {
    /** 基準の動画が属するグループ。属さないか、関連動画を読む前は undefined。 */
    group,
    /** 関連動画の次の動画。無ければ undefined。 */
    nextVideo,
    playNext,
    previous,
    next: neighbor(neighbors?.nextId),
  };
}
