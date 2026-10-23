import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { useLocation, useNavigate, useParams } from "react-router";

import { getAuthSession } from "../api/auth";
import {
  getVideoSubtitles,
  reloadForViewerChange,
  reprobeVideo,
  RequestFailed,
  type SubtitleTrack,
} from "../api/client";
import { useRelatedVideos, useVideoDetail } from "../api/useVideoDetail";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { parsePlaylistQuery } from "../playlists/playlistPlayback";
import { DetailPage } from "../ui/patterns/detail-page";
import type { AutoplayPhase } from "./AutoplayNotice";
import { usePageHandoff } from "./detailsEditor/handoff";
import { useKeyboardShortcuts } from "./keyboard";
import type { PlayerControls } from "./playerControls";
import { playerAspectRatio, playerNativeWidth } from "./aspect";
import {
  autoplayRequested,
  backTarget,
  linkStateFrom,
  resumePosition,
  startFromSearch,
  videoTrail,
} from "./pageDecisions";
import type { PlaybackFailureKind } from "./playbackRecovery";
import PlayerArrows from "./PlayerArrows";
import {
  isPlayable,
  isUnplayable,
  playerLayer,
  stallWarningShown,
  statusLayer,
  thumbnailCapture,
} from "./playerLayers";
import { useNeighborNavigation } from "./useNeighborNavigation";
import { useVideoGroupContext } from "./useVideoGroupContext";
import { useUnplayableSkip, useVideoPlaylist } from "./useVideoPlaylist";
import StallWarning from "./StallWarning";
import VideoHeader from "./VideoHeader";
import VideoPageAside from "./VideoPageAside";
import VideoPageDetails from "./VideoPageDetails";
import VideoPlayer, { initialPlayerStatus, type PlayerStatus } from "./VideoPlayer";
import { useProgressSaving } from "./useProgressSaving";
import { useVirtualThumbnail } from "./virtualVideo/useVirtualThumbnail";
import VirtualMedia from "./virtualVideo/VirtualMedia";

const noSubtitles: readonly SubtitleTrack[] = [];

/** Retry and advance recreate the player with a chosen position and autoplay state. */
interface Attempt {
  key: number;
  startMs: number | null;
  autoplay: boolean;
}

export default function VideoPage() {
  const id = videoIdParam(useParams().id);
  const location = useLocation();
  const navigate = useNavigate();
  const backTo = backTarget(location.state);
  const trail = useMemo(() => videoTrail(location.state), [location.state]);
  const linkState = useMemo(() => linkStateFrom(backTo, trail, id), [backTo, trail, id]);
  // Hide owner data and actions from guests.
  //
  const audience = useAudience();
  const owner = audience === "owner";
  //
  const playlistQuery = parsePlaylistQuery(location.search);
  const itemId = playlistQuery?.itemId ?? 0;
  // Changing playlist items also resets playback, even for the same video.
  const pageKey = `${String(id)}:${String(itemId)}`;
  // Playlist context overrides group context; smart-folder context still applies within folder items.
  //
  const groupContext = useVideoGroupContext(playlistQuery !== null);
  const { videoTo } = groupContext;
  const readContext = useMemo(
    () => ({ group: groupContext.group, smartFolder: groupContext.smartFolder }),
    [groupContext.group, groupContext.smartFolder],
  );

  const detailsHandoff = usePageHandoff(id);
  const {
    state: detailState,
    refresh,
    replace,
    setFavorite,
  } = useVideoDetail(
    id,
    groupContext.group,
    detailsHandoff.video,
    groupContext.smartFolder,
  );
  const {
    state: relatedState,
    retry: retryRelated,
    loadMore: loadMoreRelated,
    rename: renameRelated,
    rethumb: rethumbRelated,
  } = useRelatedVideos(id, groupContext.group, groupContext.smartFolder);
  const detail = currentFor(detailState, id);
  const related = currentFor(relatedState, id);
  const video = detail.kind === "ready" ? detail.video : undefined;
  const thumbnail = useVirtualThumbnail(
    video,
    (saved, mark) => {
      replace(saved, mark, "keep");
      rethumbRelated(saved);
    },
    () => void refresh(),
  );

  // Reset page and main-column scrolling on navigation; LibraryPage restores its own scroll position.
  const frameRef = useRef<HTMLDivElement | null>(null);
  useLayoutEffect(() => {
    window.scrollTo(0, 0);
    frameRef.current?.closest('[data-slot="detail-page-main"]')?.scrollTo?.(0, 0);
  }, [pageKey]);

  const [pageId, setPageId] = useState(pageKey);
  const [controls, setControls] = useState<PlayerControls | null>(null);
  const [status, setStatus] = useState<PlayerStatus>(initialPlayerStatus);
  const [failure, setFailure] = useState<{
    positionMs: number;
    kind: PlaybackFailureKind;
  } | null>(null);
  // A `t` in the address, as a face span links, wins over the saved position, also on reload.
  const linkedStartMs = startFromSearch(location.search);
  const [attempt, setAttempt] = useState<Attempt>(() => ({
    key: 0,
    startMs: linkedStartMs,
    autoplay: autoplayRequested(location.state),
  }));
  const [endedTakesFocus, setEndedTakesFocus] = useState(false);
  const [autoplayPhase, setAutoplayPhase] = useState<AutoplayPhase>("notice");
  // Dismissing a stall warning survives retries of this video, but not navigation.
  const [stallDismissedId, setStallDismissedId] = useState<number | null>(null);
  // The playback aspect ratio is more reliable than probe metadata.
  const [mediaAspect, setMediaAspect] = useState<number | null>(null);
  // Include overlays in the fullscreen container.
  const fullscreenTarget = useCallback(() => frameRef.current, []);
  const [fullscreenFrame, setFullscreenFrame] = useState<HTMLElement | null>(null);
  useEffect(() => {
    const sync = () => {
      const frame = frameRef.current;
      setFullscreenFrame(
        frame !== null && document.fullscreenElement === frame ? frame : null,
      );
    };
    document.addEventListener("fullscreenchange", sync);
    return () => document.removeEventListener("fullscreenchange", sync);
  }, []);

  // Increment on playlist refresh; settledKey identifies the completed response.
  const [playlistReadKey, setPlaylistReadKey] = useState(0);

  // A different item referencing the same video restarts it from the beginning.
  if (pageId !== pageKey) {
    const sameVideo = pageId.split(":")[0] === String(id);
    setPageId(pageKey);
    setFailure(null);
    setStatus(initialPlayerStatus);
    setEndedTakesFocus(false);
    setAutoplayPhase("notice");
    setStallDismissedId(null);
    setMediaAspect(null);
    setAttempt({
      key: 0,
      startMs: linkedStartMs ?? (sameVideo ? 0 : null),
      autoplay: autoplayRequested(location.state),
    });
  }

  // Consume autoplay once so reloads and history navigation do not restart playback.
  useEffect(() => {
    if (!autoplayRequested(location.state)) return;
    void navigate(
      { pathname: location.pathname, search: location.search },
      { replace: true, state: { from: backTo, trail } },
    );
  }, [backTo, location.pathname, location.search, location.state, navigate, trail]);

  const close = useCallback(() => void navigate(backTo), [backTo, navigate]);

  const title = video?.title;
  useEffect(() => {
    const previous = document.title;
    if (title !== undefined) document.title = t.player.documentTitle(title);
    return () => {
      document.title = previous;
    };
  }, [title]);

  // Subtitle failures must not interrupt playback or leak a previous video's tracks.
  const [subtitleState, setSubtitleState] = useState<{
    id: number;
    tracks: readonly SubtitleTrack[];
  }>({ id: 0, tracks: noSubtitles });
  useEffect(() => {
    if (id <= 0) return;
    const controller = new AbortController();
    getVideoSubtitles(id, controller.signal).then(
      (tracks) => {
        if (controller.signal.aborted) return;
        setSubtitleState({ id, tracks: Array.isArray(tracks) ? tracks : noSubtitles });
      },
      () => {
        if (controller.signal.aborted) return;
        setSubtitleState({ id, tracks: noSubtitles });
      },
    );
    return () => controller.abort();
  }, [id]);
  const subtitles = subtitleState.id === id ? subtitleState.tracks : noSubtitles;

  // Wait for positioning before the first identified save; release the watch ID after ended.
  const { rememberProgress, savePlayerProgress, markPlayed } = useProgressSaving(
    id,
    owner,
    {
      positioned: status.positioned,
      ended: status.ended,
    },
  );

  const endedRef = useRef(false);
  const onStatus = useCallback((next: PlayerStatus) => {
    // Move focus to the ended overlay only if it was inside the player.
    if (next.ended && !endedRef.current) {
      const frame = frameRef.current;
      setEndedTakesFocus(frame !== null && frame.contains(document.activeElement));
      setAutoplayPhase("notice");
      // Refresh playlist context before showing the autoplay notice.
      setPlaylistReadKey((value) => value + 1);
    }
    endedRef.current = next.ended;
    setStatus(next);
  }, []);

  // Media errors omit 401 and audience headers; verify the session before showing a playback failure.
  // Abort on video changes and ignore stale results because this route reuses its component.
  const sessionCheck = useRef<AbortController | null>(null);
  const currentId = useRef(id);
  currentId.current = id;
  useEffect(() => () => sessionCheck.current?.abort(), [id]);
  const onError = useCallback(
    (positionMs: number, kind: PlaybackFailureKind) => {
      sessionCheck.current?.abort();
      const controller = new AbortController();
      sessionCheck.current = controller;
      const checkedId = id;
      const stale = () => controller.signal.aborted || currentId.current !== checkedId;
      const showFailure = () => {
        setFailure({ positionMs, kind });
        // Refresh to detect removal or a video becoming unavailable to guests.
        void refresh();
      };
      getAuthSession(undefined, controller.signal).then(
        (session) => {
          if (stale()) return;
          if (session.state !== audience) {
            reloadForViewerChange();
            return;
          }
          showFailure();
        },
        () => {
          if (stale()) return;
          showFailure();
        },
      );
    },
    [audience, id, refresh],
  );

  const retryPlayback = () => {
    const startMs = failure?.positionMs ?? 0;
    setFailure(null);
    setStatus(initialPlayerStatus);
    setAttempt((previous) => ({ key: previous.key + 1, startMs, autoplay: true }));
  };

  const reprobe = useCallback(async () => {
    try {
      await reprobeVideo(id);
    } catch (error) {
      // A 409 means another request already started reprobe.
      if (!(error instanceof RequestFailed && error.code === "probe_not_failed"))
        throw error;
    }
    // A 202 response lacks location data, so reload instead of replacing the video.
    await refresh();
  }, [id, refresh]);

  const keepPlaying = status.playing || status.ended;
  const neighbors = useNeighborNavigation({
    related,
    trail,
    linkState,
    backTo,
    videoTo,
    keepPlaying,
  });
  const playlist = useVideoPlaylist({
    query: playlistQuery,
    videoId: id,
    owner,
    readKey: playlistReadKey,
    ended: status.ended,
    keepPlaying,
    linkState,
    smartFolder: groupContext.smartFolder,
    adoptedSmartFolder: video?.smartFolder?.id,
  });

  // Transfer focus between overlays only when it was inside the dismissed overlay.
  const leaveNotice = (phase: Exclude<AutoplayPhase, "notice">) => {
    const frame = frameRef.current;
    setEndedTakesFocus(frame !== null && frame.contains(document.activeElement));
    setAutoplayPhase(phase);
  };
  const cancelAutoplay = () => leaveNotice("cancelled");

  const shownStatus = statusLayer({
    detail,
    failure,
    onRetryLoad: refresh,
    onReprobe: owner ? reprobe : undefined,
    onRetryPlayback: retryPlayback,
  });
  const showPlayer = isPlayable(video) && detail.kind === "ready";
  const { layer, noticeShown } = playerLayer(shownStatus, {
    status,
    pageKey,
    videoId: id,
    playlist,
    group: neighbors.group,
    nextVideo: neighbors.nextVideo,
    autoplayPhase,
    linkState,
    takeFocus: endedTakesFocus,
    owner,
    controls,
    onCancel: cancelAutoplay,
    onPlayNext: neighbors.playNext,
    onGroupGone: () => leaveNotice("gone"),
    onPlaylistGone: () => setPlaylistReadKey((value) => value + 1),
    videoTo,
    nextTo: groupContext.nextTo,
  });

  // Editing owns Space and Escape while its dialog is open.
  const [editingThumbnail, setEditingThumbnail] = useState(false);

  useUnplayableSkip({
    playlist,
    autoplay: attempt.autoplay,
    unplayable: isUnplayable(detail),
    videoTitle: video?.title,
    pageKey,
    linkState,
  });

  // Escape cancels the countdown without leaving the page.
  useKeyboardShortcuts(
    showPlayer ? controls : null,
    noticeShown ? cancelAutoplay : close,
    editingThumbnail,
  );

  const aspect = playerAspectRatio(video, mediaAspect);
  return (
    <div className="min-h-dvh bg-background lg:flex lg:h-dvh lg:min-h-0 lg:flex-col lg:overflow-hidden">
      <DetailPage
        // Center player and details at the media width, bounded by aspect ratio and original display size.
        style={videoSizeStyle(aspect, playerNativeWidth(video))}
        contentClassName="lg:max-w-player-width-lg"
        header={
          <VideoHeader
            folder={video?.folder}
            virtual={thumbnail.video !== undefined}
            onClose={close}
          />
        }
        media={
          <div
            ref={frameRef}
            data-player-frame=""
            // Keep controls at least 16:9 wide while letterboxing portrait video within the frame.
            // Descendants use these variables for seek previews and player size tokens.
            // Reserve room for one title line; tags and facts scroll within the main column.
            style={{ "--vv-video-aspect": String(aspect) } as CSSProperties}
            className="relative isolate mx-auto grid w-full max-w-player-width shrink-0 grid-cols-1 overflow-hidden bg-navbar lg:max-w-player-width-lg lg:rounded-lg [&:fullscreen]:rounded-none"
          >
            {/* Aspect ratio supplies a minimum height outside fullscreen; status content may grow taller. */}
            <div
              aria-hidden="true"
              className="col-start-1 row-start-1 aspect-player max-h-player-height lg:max-h-player-height-lg [:fullscreen>&]:hidden"
            />
            {/* A virtual video has no player and no processing to report. */}
            {thumbnail.video !== undefined ? (
              <VirtualMedia video={thumbnail.video} owner={owner} thumbnail={thumbnail} />
            ) : (
              <div
                data-overlay-layer=""
                className="pointer-events-none relative z-10 col-start-1 row-start-1 flex min-w-0"
              >
                {layer}
              </div>
            )}
            {stallWarningShown({
              showPlayer,
              status,
              dismissed: stallDismissedId === id,
              statusShown: shownStatus !== null,
            }) && (
              <StallWarning
                onDismiss={() => setStallDismissedId(id)}
                container={fullscreenFrame}
              />
            )}
            {showPlayer && (
              <VideoPlayer
                key={`${pageKey}:${String(attempt.key)}`}
                video={video}
                initialPositionMs={attempt.startMs ?? resumePosition(video)}
                autoplay={attempt.autoplay}
                onPosition={rememberProgress}
                onProgress={savePlayerProgress}
                onPlay={markPlayed}
                onError={onError}
                onControls={setControls}
                onStatus={onStatus}
                onAspectRatio={setMediaAspect}
                fullscreenTarget={fullscreenTarget}
                subtitles={subtitles}
              />
            )}
            <PlayerArrows
              playlist={playlist}
              previous={neighbors.previous}
              next={neighbors.next}
              showPlayer={showPlayer}
              chromeVisible={!status.playing || status.userActive}
              ended={status.ended}
              container={fullscreenFrame}
            />
          </div>
        }
        aside={
          <VideoPageAside
            related={related}
            linkState={linkState}
            onRetry={retryRelated}
            onLoadMore={loadMoreRelated}
            readContext={readContext}
            groupMemberTarget={groupContext.memberTarget(linkState)}
            playlist={playlist}
            backTo={backTo}
            autoplay={keepPlaying}
          />
        }
      >
        <VideoPageDetails
          loading={detail.kind === "loading"}
          video={video}
          owner={owner}
          playlist={playlist}
          backTo={backTo}
          trail={trail}
          autoplay={keepPlaying}
          capture={thumbnailCapture({
            shown: owner && showPlayer,
            controls,
            status,
            statusShown: shownStatus !== null,
          })}
          detail={{ refresh, replace, setFavorite }}
          related={{ rename: renameRelated, rethumb: rethumbRelated }}
          handoff={detailsHandoff}
          thumbnail={thumbnail}
          onDeleted={close}
          // Drop group context after membership changes before reloading related results.
          onGroupChanged={() =>
            groupContext.readWithoutGroup(() => {
              void refresh();
              retryRelated();
            })
          }
          onEditingThumbnail={setEditingThumbnail}
        />
      </DetailPage>
    </div>
  );
}

/** Normalize invalid IDs to zero because NaN cannot compare equal to itself. */
function videoIdParam(raw: string | undefined): number {
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 0;
}

/** A read for a previous ID shows as loading until this ID's read settles. */
function currentFor<S extends { id: number }>(
  state: S,
  id: number,
): S | { kind: "loading"; id: number } {
  return state.id === id ? state : { kind: "loading", id };
}

function videoSizeStyle(aspect: number, nativeWidth: number | undefined): CSSProperties {
  const style: Record<string, string> = { "--vv-video-aspect": String(aspect) };
  if (nativeWidth !== undefined) style["--vv-video-width"] = `${String(nativeWidth)}px`;
  return style as CSSProperties;
}
