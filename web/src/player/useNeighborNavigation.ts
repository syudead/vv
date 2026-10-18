import { useNavigate } from "react-router";

import type { RelatedState } from "../api/useVideoDetail";
import type { Neighbor } from "./NeighborArrows";
import type { VideoLinkState } from "./pageDecisions";
import { type PreviousVideo, usePreviousVideo } from "./usePreviousVideo";

/** Group neighbors follow member order; otherwise use the previous visited video and first related result. Preserve playback intent when navigating. */
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
  // Show the previous arrow only after verifying the video remains accessible.
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
  // Pop the previous destination from history so repeated back navigation continues backward.
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
    group,
    nextVideo,
    playNext,
    previous,
    next: neighbor(neighbors?.nextId),
  };
}
