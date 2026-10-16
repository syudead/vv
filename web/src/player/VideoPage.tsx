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

const noSubtitles: readonly SubtitleTrack[] = [];

/** 再生の試み。再試行と「次を再生」は、位置と自動再生を決めてプレイヤーを作り直す。 */
interface Attempt {
  key: number;
  startMs: number | null;
  autoplay: boolean;
}

/**
 * VideoPage は動画詳細画面（`/videos/:id`）である。
 *
 * 構成要素は見出しの帯（ロゴ・置き場所のパンくず・×）・プレイヤー・題名・タグ・
 * ファイルの情報・関連動画とする（親 Issue 要件 1・3、issue 268、ui-design「Video header」
 * 「Video facts」）。状態と失敗は、プレイヤーの上の 1 つの入れ物に重ねて伝える
 * （plan の Structural Decisions 12）。状態表示と再生終了は同じ入れ物に出す。
 */
export default function VideoPage() {
  const params = useParams();
  // 形の正しくない id は 0 に寄せる（NaN は自分自身と等しくならず、比べられない）。
  const parsedId = Number(params.id);
  const id = Number.isSafeInteger(parsedId) && parsedId > 0 ? parsedId : 0;
  const location = useLocation();
  const navigate = useNavigate();
  const backTo = backTarget(location.state);
  // 動画詳細画面の間で移ってきた道筋。最後が直前に見ていた動画である（issue 931）。
  const trail = useMemo(() => videoTrail(location.state), [location.state]);
  // この画面から別の動画へ移るリンクに載せる状態。
  const linkState = useMemo(() => linkStateFrom(backTo, trail, id), [backTo, trail, id]);
  // ゲストには所有者のデータ（タグ・ファイルの場所・再生位置）と所有者だけの操作
  // （読み取りのやり直し・既定アプリで開く）を出さない
  // （specs/016-single-account-auth/ui-design.md「Guest degradation」）。
  const audience = useAudience();
  const owner = audience === "owner";
  // プレイリストの文脈（`?playlist=&item=`）。所有者だけが読む
  // （specs/044-playlists/ui-design.md「Video page in a playlist」、research.md R-8）。
  const playlistQuery = parsePlaylistQuery(location.search);
  const itemId = playlistQuery?.itemId ?? 0;
  // 別の項目へ移ったときも、別の動画へ移ったときと同じに扱う（R-7）。
  const pageKey = `${String(id)}:${String(itemId)}`;
  // 動画を開いたグループ（`?group`）かスマートフォルダ（`?smartFolder`）。読み取りに渡し、行・矢印・
  // メンバーの並び・予告をその再生順に従わせる。プレイリストの文脈はグループより優先するので、
  // その間は `?group` を使わない。`?smartFolder` はフォルダの項目の中でも読み取りに渡す
  // （specs/048-groups/ui-design.md「Video page」、research.md R-9、
  // specs/052-smart-folders/ui-design.md「Video page」「Video page in a playlist」）。
  const groupContext = useVideoGroupContext(playlistQuery !== null);
  const { videoTo } = groupContext;
  const readContext = useMemo(
    () => ({ group: groupContext.group, smartFolder: groupContext.smartFolder }),
    [groupContext.group, groupContext.smartFolder],
  );

  // 編集画面から戻ったときの控え（保存の応答と、入口へフォーカスを戻すこと）。
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
  const detail = detailState.id === id ? detailState : { kind: "loading" as const, id };
  const related =
    relatedState.id === id ? relatedState : { kind: "loading" as const, id };
  const video = detail.kind === "ready" ? detail.video : undefined;

  // 一覧をスクロールした位置から来ても、プレイヤーを画面の上に出す。別の動画へ移ったときも
  // 同じ。一覧へ戻ったときの位置の復元は一覧の側（LibraryPage）が行う。広い画面では左右の列が
  // それぞれスクロールするので、プレイヤーのある左の列（詳細ページの主領域）も先頭へ戻す。
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
  const [attempt, setAttempt] = useState<Attempt>(() => ({
    key: 0,
    startMs: null,
    autoplay: autoplayRequested(location.state),
  }));
  const [endedTakesFocus, setEndedTakesFocus] = useState(false);
  const [autoplayPhase, setAutoplayPhase] = useState<AutoplayPhase>("notice");
  // 途切れの警告を閉じた動画の id。同じ動画の再生の間（失敗からの再試行を含む）は出し直さず、
  // 別の動画へ移れば忘れる（specs/027-playback-quality/research.md R-7、要件 10）。
  const [stallDismissedId, setStallDismissedId] = useState<number | null>(null);
  // 再生を始めて分かった映像の比率。解析の値より確かなので、分かればこちらを使う。
  const [mediaAspect, setMediaAspect] = useState<number | null>(null);
  // 全画面はプレイヤーの上の層ごとにする（状態表示・再生終了も全画面で出す）。
  const fullscreenTarget = useCallback(() => frameRef.current, []);
  // 全画面にしている入れ物。前後の矢印の吹き出しは、その間だけ入れ物の中に描く。
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

  // プレイリストの再生の応答を読み直すたびに増やす（動画の終わり、予告が次の動画の消えたのを
  // 知ったとき）。usePlaylistContext は読み終えた値を settledKey で返す。
  const [playlistReadKey, setPlaylistReadKey] = useState(0);

  // 別の動画（または同じ動画の別の項目）へ移ったら、前の再生の状態を持ち越さない。
  // 動画が同じで項目だけ違うときは、その動画を始めから再生し直す（R-7）。
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
      startMs: sameVideo ? 0 : null,
      autoplay: autoplayRequested(location.state),
    });
  }

  // 「次を再生」の自動再生は 1 回だけ使う。再読み込みや履歴の戻りで再生を始めない。
  // 見てきた道筋は残す。
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

  // --- 字幕 ---
  // 動画を開くたびに隣の字幕の一覧を取り直す（specs/028-sidecar-subtitles contracts §3）。
  // 取れなければ字幕無しとして扱い、再生は止めない。前の動画の一覧は次の動画に渡さない。
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

  // --- 再生位置の保存と、視聴履歴の視聴の識別子 ---
  // 識別子を付けた最初の保存は positioned を待ち、ended のあとで識別子を捨てる。
  const { rememberProgress, savePlayerProgress, markPlayed } = useProgressSaving(
    id,
    owner,
    {
      positioned: status.positioned,
      ended: status.ended,
    },
  );

  // --- プレイヤーの状態 ---
  const endedRef = useRef(false);
  const onStatus = useCallback((next: PlayerStatus) => {
    // 再生終了の層へフォーカスを移すのは、フォーカスがプレイヤーの中にあったときだけにする。
    if (next.ended && !endedRef.current) {
      const frame = frameRef.current;
      setEndedTakesFocus(frame !== null && frame.contains(document.activeElement));
      // 終わるたびに、グループのメンバーなら予告から始める。
      setAutoplayPhase("notice");
      // プレイリストの文脈では、予告を出す前に再生の応答を読み直す（R-6）。
      setPlaylistReadKey((value) => value + 1);
    }
    endedRef.current = next.ended;
    setStatus(next);
  }, []);

  // 再生の失敗は、見る人が変わった（セッションが失効した）せいかもしれない。`video`
  // 要素の読み込みの失敗には 401 も X-VV-Audience も付かないので、状態を確かめ、
  // 変わっていれば失敗の層を出さずにページを1度だけ読み直す（ui-design.md
  // 「Guest degradation」）。変わっていなければ、今の再生失敗の層を出す。
  // 同じ route で別の動画へ移るとこの部品は使い回されるので、確かめは動画が変わったときにも
  // 止め、返ってきたときに始めた動画のままのときだけ結果を使う。前の動画の失敗を次の動画に
  // 出さないためである。
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
        // 動画がライブラリから消えた（ゲストでは公開でなくなった）せいかもしれない。
        // 取り直して確かめる。
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
      // 409 は、別のタブや連打ですでにやり直しが始まっている。
      if (!(error instanceof RequestFailed && error.code === "probe_not_failed"))
        throw error;
    }
    // 202 の本文は所在を持たないので、置き換えずに取り直す。
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

  // 予告を取り消したら今の再生終了の層を出す。フォーカスが予告の中にあったなら、
  // 次の層の主な操作（「次を再生」）へ移す。
  // 次のメンバーが消えたときも同じく、「もう一度見る」だけの層へフォーカスを渡す。
  const leaveNotice = (phase: Exclude<AutoplayPhase, "notice">) => {
    const frame = frameRef.current;
    setEndedTakesFocus(frame !== null && frame.contains(document.activeElement));
    setAutoplayPhase(phase);
  };
  const cancelAutoplay = () => leaveNotice("cancelled");

  // --- プレイヤーの上に重ねる層（同時に 1 つだけ） ---
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
    // 次の動画が消えていたら、再生の応答を読み直して予告を移す。
    onPlaylistGone: () => setPlaylistReadKey((value) => value + 1),
    videoTo,
    nextTo: groupContext.nextTo,
  });

  // サムネイルの編集画面が開いている間は、この画面のキーを効かせない。Space は欄に入り、
  // Esc は編集画面のものになる（specs/045-comfyui-thumbnails/ui-design.md「Closing」）。
  const [editingThumbnail, setEditingThumbnail] = useState(false);

  useUnplayableSkip({
    playlist,
    autoplay: attempt.autoplay,
    unplayable: isUnplayable(detail),
    videoTitle: video?.title,
    pageKey,
    linkState,
  });

  // 予告の間の Esc は取り消しにし、画面を閉じない（ui-design.md「Autoplay notice」）。
  useKeyboardShortcuts(
    showPlayer ? controls : null,
    noticeShown ? cancelAutoplay : close,
    editingThumbnail,
  );

  const aspect = playerAspectRatio(video, mediaAspect);
  return (
    // 詳細ページの型（DetailPage）で組む。見出しの帯の下に、左の列（プレイヤー・題名・タグ・
    // 公開・情報）と右の列（関連動画）を並べる（web/registry/rules/patterns.md の Detail page）。
    // 広い画面はページを画面の高さに留め、左右の列がそれぞれ中でスクロールする。
    <div className="min-h-dvh bg-background lg:flex lg:h-dvh lg:min-h-0 lg:flex-col lg:overflow-hidden">
      <DetailPage
        // 広い画面では、プレイヤーが高さか幅の上限で止まって左の列より細いとき、プレイヤーと
        // 題名・情報をプレイヤーの幅にそろえて列の中央に置く。幅は動画の比率と元の表示幅から
        // 決まる（大きな画面で小さな動画を引き延ばしすぎない）ので、両方を渡す。
        style={videoSizeStyle(aspect, playerNativeWidth(video))}
        contentClassName="lg:max-w-player-width-lg"
        header={<VideoHeader folder={video?.folder} onClose={close} />}
        media={
          <div
            ref={frameRef}
            data-player-frame=""
            // 枠の幅は 16:9 の動画と同じ（それより横長なら動画の比率）で、高さは動画の比率に
            // 合わせて画面に収まるまで伸ばす。縦長の動画は枠の中央に左右の余白付きで出るので、
            // 前後の動画へのつまみと操作バーは横長のときと同じ位置・幅のままになる。
            // 比率はシークのプレビューと枠の大きさの段（player-width・player-height、
            // src/ui/tokens.css）も使うので、変数として子孫へ渡す。
            // 広い画面では、題名の 1 行までが左の列に収まる高さを上限にし、横長の動画は列の幅
            // いっぱいまで広げる。タグと情報の行は列をスクロールして見る。上限で列より細く
            // なったときは、中身ごと列の中央に置く（DetailPage の contentClassName）。
            style={{ "--vv-video-aspect": String(aspect) } as CSSProperties}
            className="relative isolate mx-auto grid w-full max-w-player-width shrink-0 grid-cols-1 overflow-hidden bg-navbar lg:max-w-player-width-lg lg:rounded-lg [&:fullscreen]:rounded-none"
          >
            {/* 動画の比率（画面の高さまで）は下限。状態表示が収まらない幅では、内容に合わせて伸びる。
              全画面では入れ物が画面いっぱいになるので、下限は要らない。 */}
            <div
              aria-hidden="true"
              className="col-start-1 row-start-1 aspect-player max-h-player-height lg:max-h-player-height-lg [:fullscreen>&]:hidden"
            />
            <div
              data-overlay-layer=""
              className="pointer-events-none relative z-10 col-start-1 row-start-1 flex min-w-0"
            >
              {layer}
            </div>
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
          // グループの操作（外す・タグ化・解散）の後に、動画と関連動画を `group` なしで読み直す。
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

/**
 * videoSizeStyle は詳細ページへ渡す動画の比率と元の表示幅の変数である。元の表示幅が
 * 分からなければ比率だけ渡す。
 */
function videoSizeStyle(aspect: number, nativeWidth: number | undefined): CSSProperties {
  const style: Record<string, string> = { "--vv-video-aspect": String(aspect) };
  if (nativeWidth !== undefined) style["--vv-video-width"] = `${String(nativeWidth)}px`;
  return style as CSSProperties;
}
