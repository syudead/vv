import type { ReactNode } from "react";

import type { Video } from "../api/client";
import type { VideoDetailState } from "../api/useVideoDetail";
import { t } from "../i18n";
import { playlistStepPath } from "../playlists/playlistPlayback";
import AutoplayNotice, { type AutoplayPhase } from "./AutoplayNotice";
import EndedOverlay from "./EndedOverlay";
import type { VideoLinkState } from "./pageDecisions";
import type { PlaybackFailureKind } from "./playbackRecovery";
import type { PlayerControls } from "./playerControls";
import PlaylistEndOverlay from "./PlaylistEndOverlay";
import {
  LoadFailure,
  LoadingOverlay,
  MissingVideo,
  PlaybackFailure,
  ProcessingStages,
  ReadFailure,
  Unplayable,
} from "./StatusOverlays";
import type { VideoPlaylist } from "./useVideoPlaylist";
import { canStartPlayback, type PlayerStatus } from "./VideoPlayer";

// Status and ended overlays share one container; display at most one.

export function isPlayable(video: Video | undefined): video is Video {
  return video !== undefined && video.probeState === "done" && canStartPlayback(video);
}

/** Unplayable states never complete automatically. */
export function isUnplayable(detail: VideoDetailState): boolean {
  if (detail.kind === "missing") return true;
  if (detail.kind !== "ready") return false;
  const { probeState } = detail.video;
  return probeState === "failed" || (probeState === "done" && !isPlayable(detail.video));
}

export function statusLayer({
  detail,
  failure,
  onRetryLoad,
  onReprobe,
  onRetryPlayback,
}: {
  detail: VideoDetailState;
  failure: { positionMs: number; kind: PlaybackFailureKind } | null;
  onRetryLoad: () => Promise<void>;
  onReprobe: (() => Promise<void>) | undefined;
  onRetryPlayback: () => void;
}): ReactNode {
  if (detail.kind === "missing") return <MissingVideo />;
  if (detail.kind === "failed")
    return <LoadFailure reason={detail.reason} onRetry={onRetryLoad} />;
  if (detail.kind === "loading") return <LoadingOverlay backdrop />;
  const video = detail.video;
  if (video.probeState === "pending") return <ProcessingStages video={video} />;
  if (video.probeState === "failed")
    return <ReadFailure video={video} onReprobe={onReprobe} />;
  if (!isPlayable(video)) return <Unplayable />;
  if (failure !== null)
    return (
      <PlaybackFailure
        positionMs={failure.positionMs}
        kind={failure.kind}
        onRetry={onRetryPlayback}
      />
    );
  return null;
}

export interface EndLayerArgs {
  status: PlayerStatus;
  pageKey: string;
  videoId: number;
  playlist: VideoPlaylist;
  group: unknown;
  nextVideo: Video | undefined;
  autoplayPhase: AutoplayPhase;
  linkState: VideoLinkState;
  takeFocus: boolean;
  owner: boolean;
  controls: PlayerControls | null;
  onCancel: () => void;
  onPlayNext: () => void;
  onGroupGone: () => void;
  onPlaylistGone: () => void;
  videoTo: (videoId: number) => string;
  nextTo: (next: { id: number } | undefined) => string | undefined;
}

export interface PlayerLayer {
  layer: ReactNode;
  noticeShown: boolean;
}

const noLayer: PlayerLayer = { layer: null, noticeShown: false };

export function endLayer(args: EndLayerArgs): PlayerLayer {
  const { status, playlist } = args;
  if (playlist.reading)
    return { layer: <LoadingOverlay backdrop={false} />, noticeShown: false };
  const playlistEnded = status.ended && playlist.context.status === "ready";
  if (!playlistEnded) return groupEndLayer(args);
  const next = playlist.next;
  const replay = () => args.controls?.restart();
  if (next !== undefined && args.autoplayPhase === "notice")
    return {
      layer: (
        <AutoplayNotice
          key={`${args.pageKey}:${String(next.itemId)}:${String(playlist.context.settledKey)}`}
          next={next.video}
          to={playlistStepPath(playlist.playlistId, next)}
          linkState={args.linkState}
          takeFocus={args.takeFocus}
          watchEvents={args.owner}
          onCancel={args.onCancel}
          onPlayNow={playlist.playNext}
          onAdvance={playlist.playNext}
          onGone={args.onPlaylistGone}
        />
      ),
      noticeShown: true,
    };
  if (next !== undefined)
    return {
      layer: (
        <EndedOverlay
          next={next.video}
          to={playlistStepPath(playlist.playlistId, next)}
          linkState={args.linkState}
          takeFocus={args.takeFocus}
          onReplay={replay}
          onPlayNext={playlist.playNext}
        />
      ),
      noticeShown: false,
    };
  if (playlist.playback !== undefined)
    return {
      layer: (
        <PlaylistEndOverlay
          name={playlist.playback.playlist.name}
          takeFocus={args.takeFocus}
          onReplay={replay}
        />
      ),
      noticeShown: false,
    };
  return groupEndLayer(args);
}

function groupEndLayer(args: EndLayerArgs): PlayerLayer {
  const { status, playlist, nextVideo } = args;
  const outsidePlaylist = !playlist.inPlaylist && !playlist.dropped;
  if (!status.ended) return loadingLayer(status);
  if (
    outsidePlaylist &&
    args.group !== undefined &&
    nextVideo !== undefined &&
    args.autoplayPhase === "notice"
  )
    return {
      layer: (
        <AutoplayNotice
          key={`${String(args.videoId)}:${String(nextVideo.id)}`}
          next={nextVideo}
          to={args.videoTo(nextVideo.id)}
          linkState={args.linkState}
          takeFocus={args.takeFocus}
          watchEvents={args.owner}
          onCancel={args.onCancel}
          onPlayNow={args.onPlayNext}
          onAdvance={args.onPlayNext}
          onGone={args.onGroupGone}
        />
      ),
      noticeShown: true,
    };
  return {
    layer: (
      <EndedOverlay
        // Playlist context, even after dropping it, must not advance into related videos.
        next={args.autoplayPhase === "gone" || !outsidePlaylist ? undefined : nextVideo}
        to={args.nextTo(nextVideo)}
        linkState={args.linkState}
        takeFocus={args.takeFocus}
        onReplay={() => args.controls?.restart()}
        onPlayNext={args.onPlayNext}
      />
    ),
    noticeShown: false,
  };
}

function loadingLayer(status: PlayerStatus): PlayerLayer {
  if (status.reconnecting)
    return {
      layer: <LoadingOverlay backdrop={false} label={t.player.reconnecting} />,
      noticeShown: false,
    };
  if (status.loading)
    return { layer: <LoadingOverlay backdrop={false} />, noticeShown: false };
  return noLayer;
}

export function playerLayer(status: ReactNode, args: EndLayerArgs): PlayerLayer {
  return status === null ? endLayer(args) : { layer: status, noticeShown: false };
}

/** Suppress stall warnings behind terminal, autoplay, reconnecting, or processing overlays; loading spinners may coexist. */
export function stallWarningShown({
  showPlayer,
  status,
  dismissed,
  statusShown,
}: {
  showPlayer: boolean;
  status: PlayerStatus;
  dismissed: boolean;
  statusShown: boolean;
}): boolean {
  return (
    showPlayer &&
    status.stalled &&
    !dismissed &&
    !statusShown &&
    !status.ended &&
    !status.reconnecting
  );
}

/** Enable owner-only capture after positioning, with no status, ended, or autoplay overlay obscuring the frame. */
/** Read logical playback milliseconds at click time; pause playback before editing. */
export interface ThumbnailCapture {
  enabled: boolean;
  positionMs: () => number | null;
  pause: () => void;
}

export function thumbnailCapture({
  shown,
  controls,
  status,
  statusShown,
}: {
  shown: boolean;
  controls: PlayerControls | null;
  status: PlayerStatus;
  statusShown: boolean;
}): ThumbnailCapture | undefined {
  if (!shown) return undefined;
  return {
    enabled: controls !== null && status.positioned && !statusShown && !status.ended,
    positionMs: () => controls?.positionMs() ?? null,
    pause: () => controls?.pause(),
  };
}
