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
import type { ThumbnailCapture } from "./VideoFacts";
import { canStartPlayback, type PlayerStatus } from "./VideoPlayer";

// 動画ページのプレイヤーの上に重ねる層の決め方（同時に 1 つだけ。plan の Structural
// Decisions 12）。状態表示と再生終了は同じ入れ物に出す。

/** isPlayable は動画を読み終え、プレイヤーで再生を始められるかである。 */
export function isPlayable(video: Video | undefined): video is Video {
  return video !== undefined && video.probeState === "done" && canStartPlayback(video);
}

/**
 * isUnplayable は再生のできない状態（読み取りの失敗・再生できない・動画が無い）かである。
 * この状態は終わらない（ui-design.md「Unplayable item」）。
 */
export function isUnplayable(detail: VideoDetailState): boolean {
  if (detail.kind === "missing") return true;
  if (detail.kind !== "ready") return false;
  const { probeState } = detail.video;
  return probeState === "failed" || (probeState === "done" && !isPlayable(detail.video));
}

/** statusLayer は動画の状態と再生の失敗の層である。何も伝えないときは null。 */
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
  /** 読み取りのやり直し。所有者でなければ undefined。 */
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

/** EndLayerArgs は再生の終わりと読み込み中の層を決める値である。 */
export interface EndLayerArgs {
  status: PlayerStatus;
  pageKey: string;
  videoId: number;
  playlist: VideoPlaylist;
  /** 基準の動画が属するグループ（関連動画の応答）。 */
  group: unknown;
  /** 関連動画の次の動画。 */
  nextVideo: Video | undefined;
  autoplayPhase: AutoplayPhase;
  linkState: VideoLinkState;
  takeFocus: boolean;
  owner: boolean;
  controls: PlayerControls | null;
  /** 予告を取り消す。 */
  onCancel: () => void;
  /** 関連動画の次を自動再生で開く。 */
  onPlayNext: () => void;
  /** グループの次のメンバーが消えた。 */
  onGroupGone: () => void;
  /** プレイリストの次の動画が消えた。再生の応答を読み直す。 */
  onPlaylistGone: () => void;
  videoTo: (videoId: number) => string;
  nextTo: (next: { id: number } | undefined) => string | undefined;
}

/** PlayerLayer は重ねる層と、それが次の予告か（予告の間の Esc は取り消し）である。 */
export interface PlayerLayer {
  layer: ReactNode;
  noticeShown: boolean;
}

const noLayer: PlayerLayer = { layer: null, noticeShown: false };

/**
 * endLayer は状態の層が無いときの層である。プレイリストの文脈では、次の動画があれば予告を、
 * 無ければ終わりの層を出す（ui-design.md「Video page in a playlist」）。それ以外は
 * groupEndLayer が決める。
 */
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

/**
 * groupEndLayer はプレイリストの層の外の終わりと読み込み中の層である。グループのメンバーで
 * 次のメンバーがあるときは、今の再生終了の層の代わりに予告を出す。グループに属さない動画は
 * 今のまま（受け入れ条件 17）。
 */
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
        // プレイリストの文脈（落とした後を含む）では、関連動画の次を出さない（R-7）。
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

/** loadingLayer は再生中の再接続とデータ待ちの輪である。 */
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

/**
 * playerLayer はプレイヤーの上に重ねる層である。状態の層（statusLayer）があればそれを出し、
 * 無ければ endLayer が決める。
 */
export function playerLayer(status: ReactNode, args: EndLayerArgs): PlayerLayer {
  return status === null ? endLayer(args) : { layer: status, noticeShown: false };
}

/**
 * stallWarningShown は途切れの警告を出すかである。警告は入れ物とは別の層で、失敗・再生終了・
 * 次の予告・再接続中・取り込み中の層が出ている間は出さない。データ待ちの読み込み中の輪とは
 * 並べて出す（specs/027-playback-quality/research.md R-7）。
 */
export function stallWarningShown({
  showPlayer,
  status,
  dismissed,
  statusShown,
}: {
  showPlayer: boolean;
  status: PlayerStatus;
  /** 今の動画で警告を閉じた。 */
  dismissed: boolean;
  /** 状態の層が出ている。 */
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

/**
 * thumbnailCapture は今の場面を代表サムネイルにするボタンである。所有者でプレイヤーが出て
 * いるときだけ置く。押せるのは、最初の読み込みで論理上の位置が確定し、映像を覆う状態の層・
 * 再生終了の層（と次の予告）が無いときである
 * （specs/029-video-overrides/ui-design.md「Capture button」）。
 */
export function thumbnailCapture({
  shown,
  controls,
  status,
  statusShown,
}: {
  /** 所有者でプレイヤーが出ている。 */
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
