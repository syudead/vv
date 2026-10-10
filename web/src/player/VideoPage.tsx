import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
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
import { DetailPage } from "../ui/patterns/detail-page";
import { Skeleton } from "../ui/shadcn/skeleton";
import AutoplayNotice, { type AutoplayPhase } from "./AutoplayNotice";
import EndedOverlay from "./EndedOverlay";
import GroupLine from "./GroupLine";
import NeighborArrows from "./NeighborArrows";
import { useKeyboardShortcuts } from "./keyboard";
import type { PlayerControls } from "./playerControls";
import { playerAspectRatio } from "./aspect";
import { autoplayRequested, backTarget, resumePosition } from "./pageDecisions";
import type { PlaybackFailureKind } from "./playbackRecovery";
import RelatedVideos from "./RelatedVideos";
import StallWarning from "./StallWarning";
import {
  CreatingLine,
  LoadFailure,
  LoadingOverlay,
  MissingVideo,
  PlaybackFailure,
  ProcessingStages,
  ReadFailure,
  Unplayable,
} from "./StatusOverlays";
import VideoFacts, { type ThumbnailCapture } from "./VideoFacts";
import VideoHeader from "./VideoHeader";
import VideoPlayer, {
  canStartPlayback,
  initialPlayerStatus,
  type PlayerStatus,
} from "./VideoPlayer";
import AutoTagButton from "./AutoTagButton";
import VideoTags from "./VideoTags";
import VideoTitle from "./VideoTitle";
import { useProgressSaving } from "./useProgressSaving";
import VisibilitySwitch from "./VisibilitySwitch";

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
  // ゲストには所有者のデータ（タグ・ファイルの場所・再生位置）と所有者だけの操作
  // （読み取りのやり直し・既定アプリで開く）を出さない
  // （specs/016-single-account-auth/ui-design.md「Guest degradation」）。
  const audience = useAudience();
  const owner = audience === "owner";

  const { state: detailState, refresh, replace, setFavorite } = useVideoDetail(id);
  const {
    state: relatedState,
    retry: retryRelated,
    rename: renameRelated,
    rethumb: rethumbRelated,
  } = useRelatedVideos(id);
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
  }, [id]);

  const [pageId, setPageId] = useState(id);
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

  // 別の動画へ移ったら、前の動画の再生の状態を持ち越さない。
  if (pageId !== id) {
    setPageId(id);
    setFailure(null);
    setStatus(initialPlayerStatus);
    setEndedTakesFocus(false);
    setAutoplayPhase("notice");
    setStallDismissedId(null);
    setMediaAspect(null);
    setAttempt({ key: 0, startMs: null, autoplay: autoplayRequested(location.state) });
  }

  // 「次を再生」の自動再生は 1 回だけ使う。再読み込みや履歴の戻りで再生を始めない。
  useEffect(() => {
    if (!autoplayRequested(location.state)) return;
    void navigate(
      { pathname: location.pathname, search: location.search },
      { replace: true, state: { from: backTo } },
    );
  }, [backTo, location.pathname, location.search, location.state, navigate]);

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

  // --- 再生位置の保存（既存どおり） ---
  const { rememberProgress, savePlayerProgress } = useProgressSaving(id, owner);

  // --- プレイヤーの状態 ---
  const endedRef = useRef(false);
  const onStatus = useCallback((next: PlayerStatus) => {
    // 再生終了の層へフォーカスを移すのは、フォーカスがプレイヤーの中にあったときだけにする。
    if (next.ended && !endedRef.current) {
      const frame = frameRef.current;
      setEndedTakesFocus(frame !== null && frame.contains(document.activeElement));
      // 終わるたびに、グループのメンバーなら予告から始める。
      setAutoplayPhase("notice");
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

  // グループのメンバーなら、前後はグループの中の並びで、題名はメンバーの並びから引く
  // （関連動画の items は同じグループのメンバーを含まない）。
  const neighbors = related.kind === "ready" ? related.related : undefined;
  const group = neighbors?.group;
  const findVideo = (targetId: number) =>
    (group?.items ?? neighbors?.items)?.find((item) => item.id === targetId);
  const nextVideo =
    neighbors?.nextId !== undefined ? findVideo(neighbors.nextId) : undefined;

  const playNext = () => {
    if (nextVideo === undefined) return;
    void navigate(`/videos/${String(nextVideo.id)}`, {
      state: { from: backTo, autoplay: true },
    });
  };

  // 左右の端の前後の動画。再生中（または見終えた後）に移ったときは、移った先でも再生を続ける。
  const neighbor = (targetId: number | undefined) =>
    targetId === undefined
      ? undefined
      : {
          title: findVideo(targetId)?.title,
          go: () =>
            void navigate(`/videos/${String(targetId)}`, {
              state: { from: backTo, autoplay: status.playing || status.ended },
            }),
        };

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
  const playable =
    video !== undefined && video.probeState === "done" && canStartPlayback(video);
  let statusLayer: ReactNode = null;
  if (detail.kind === "missing") statusLayer = <MissingVideo />;
  else if (detail.kind === "failed")
    statusLayer = <LoadFailure reason={detail.reason} onRetry={refresh} />;
  else if (video === undefined) statusLayer = <LoadingOverlay backdrop />;
  else if (video.probeState === "pending")
    statusLayer = <ProcessingStages video={video} />;
  else if (video.probeState === "failed")
    statusLayer = <ReadFailure video={video} onReprobe={owner ? reprobe : undefined} />;
  else if (!playable) statusLayer = <Unplayable />;
  else if (failure !== null)
    statusLayer = (
      <PlaybackFailure
        positionMs={failure.positionMs}
        kind={failure.kind}
        onRetry={retryPlayback}
      />
    );

  const showPlayer = playable && detail.kind === "ready";
  const aspect = playerAspectRatio(video, mediaAspect);
  const chromeVisible = !status.playing || status.userActive;
  let layer: ReactNode = statusLayer;
  // グループのメンバーで次のメンバーがあるときは、今の再生終了の層の代わりに予告を出す。
  // グループに属さない動画は今のまま（受け入れ条件 17）。
  const noticeShown =
    layer === null &&
    status.ended &&
    group !== undefined &&
    nextVideo !== undefined &&
    autoplayPhase === "notice";
  if (noticeShown) {
    layer = (
      <AutoplayNotice
        key={`${String(id)}:${String(nextVideo.id)}`}
        next={nextVideo}
        backTo={backTo}
        takeFocus={endedTakesFocus}
        watchEvents={owner}
        onCancel={cancelAutoplay}
        onPlayNow={playNext}
        onAdvance={playNext}
        onGone={() => leaveNotice("gone")}
      />
    );
  } else if (layer === null && status.ended) {
    layer = (
      <EndedOverlay
        next={autoplayPhase === "gone" ? undefined : nextVideo}
        backTo={backTo}
        takeFocus={endedTakesFocus}
        onReplay={() => controls?.restart()}
        onPlayNext={playNext}
      />
    );
  } else if (layer === null && status.reconnecting) {
    layer = <LoadingOverlay backdrop={false} label={t.player.reconnecting} />;
  } else if (layer === null && status.loading) {
    layer = <LoadingOverlay backdrop={false} />;
  }

  // 途切れの警告は入れ物とは別の層で、失敗・再生終了・次の予告・再接続中・取り込み中の
  // 層が出ている間は出さない。データ待ちの読み込み中の輪とは並べて出す（R-7）。
  const stallWarningShown =
    showPlayer &&
    status.stalled &&
    stallDismissedId !== id &&
    statusLayer === null &&
    !status.ended &&
    !status.reconnecting;

  // 今の場面を代表サムネイルにするボタンは、所有者でプレイヤーが出ているときだけ置く。
  // 押せるのは、最初の読み込みで論理上の位置が確定し、映像を覆う状態の層・再生終了の層
  // （と次の予告）が無いときである（specs/029-video-overrides/ui-design.md「Capture button」）。
  const capture: ThumbnailCapture | undefined =
    owner && showPlayer
      ? {
          enabled:
            controls !== null &&
            status.positioned &&
            statusLayer === null &&
            !status.ended,
          positionMs: () => controls?.positionMs() ?? null,
        }
      : undefined;

  // 予告の間の Esc は取り消しにし、画面を閉じない（ui-design.md「Autoplay notice」）。
  useKeyboardShortcuts(
    showPlayer ? controls : null,
    noticeShown ? cancelAutoplay : close,
  );

  return (
    // 詳細ページの型（DetailPage）で組む。見出しの帯の下に、左の列（プレイヤー・題名・タグ・
    // 公開・情報）と右の列（関連動画）を並べる（web/registry/rules/patterns.md の Detail page）。
    // 広い画面はページを画面の高さに留め、左右の列がそれぞれ中でスクロールする。
    <div className="min-h-dvh bg-background lg:flex lg:h-dvh lg:min-h-0 lg:flex-col lg:overflow-hidden">
      <DetailPage
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
            // 広い画面では、題名・タグ・情報の 2 行までが左の列に収まる高さを上限にし、ふだんは
            // 列をスクロールさせない。上限で列より細くなったときは、題名と左端をそろえるため
            // 左に寄せる。
            style={{ "--vv-video-aspect": String(aspect) } as CSSProperties}
            className="relative isolate mx-auto grid w-full max-w-player-width shrink-0 grid-cols-1 overflow-hidden bg-navbar lg:ml-0 lg:max-w-player-width-lg lg:rounded-lg [&:fullscreen]:rounded-none"
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
            {stallWarningShown && (
              <StallWarning
                onDismiss={() => setStallDismissedId(id)}
                container={fullscreenFrame}
              />
            )}
            {showPlayer && (
              <VideoPlayer
                key={`${String(id)}:${String(attempt.key)}`}
                video={video}
                initialPositionMs={attempt.startMs ?? resumePosition(video)}
                autoplay={attempt.autoplay}
                onPosition={rememberProgress}
                onProgress={savePlayerProgress}
                onError={onError}
                onControls={setControls}
                onStatus={onStatus}
                onAspectRatio={setMediaAspect}
                fullscreenTarget={fullscreenTarget}
                subtitles={subtitles}
              />
            )}
            {showPlayer && (
              <NeighborArrows
                previous={neighbor(neighbors?.prevId)}
                next={neighbor(neighbors?.nextId)}
                visible={chromeVisible || status.ended}
                container={fullscreenFrame}
              />
            )}
          </div>
        }
        aside={<RelatedVideos state={related} backTo={backTo} onRetry={retryRelated} />}
      >
        {detail.kind === "loading" && (
          <div aria-hidden="true" className="flex flex-col gap-3">
            <Skeleton className="h-8 w-2/3" />
            <Skeleton className="h-5 w-1/2" />
          </div>
        )}
        {video !== undefined && (
          <>
            <CreatingLine video={video} />
            {/* 題名とタグは1つのまとまり（ui-design.md「Video page tags」Placement）。 */}
            <div className="flex flex-col gap-2">
              {video.group !== undefined && (
                <GroupLine
                  group={video.group}
                  owner={owner}
                  onChanged={() => {
                    // group が無くなるので、この行・メンバーの並び・前後が
                    // ふつうの動画の形に戻る（ui-design.md「Group line」）。
                    void refresh();
                    retryRelated();
                  }}
                />
              )}
              <VideoTitle
                // 別の動画へ移ったら、編集中の入力と失敗の行を持ち越さない。
                key={`title:${String(video.id)}`}
                video={video}
                owner={owner}
                onSaved={(saved, mark) => {
                  replace(saved, mark);
                  // グループの並びにあるこの動画の題名も、画面の題名とそろえる。
                  renameRelated(saved);
                }}
                onStale={() => void refresh()}
              />
              {owner && (
                <VideoTags
                  // VideoPage 自身が動画ごとに作り直されず（同じ /videos/:id
                  // ルートのまま次の動画へ移ることがある）使い回されるため、
                  // VideoTags を videoId で作り直し、前の動画の重ねた
                  // 付け外し（appliedRef）を持ち越さない（Devin の指摘1）。
                  key={`tags:${String(video.id)}`}
                  videoId={video.id}
                  tags={video.tags}
                  onStaleVideo={() => void refresh()}
                  // 更新日時が進むので取り直す。取り直しの間と失敗したときは前の値のまま
                  // （specs/033-video-dates/ui-design.md「Refresh after edits」）。
                  onChanged={() => void refresh()}
                />
              )}
              {owner && (
                // タグの並びのすぐ下で、この動画のタグを判定モデルに聞く
                // （docs/design-docs/auto-tagging.md）。
                <AutoTagButton key={`auto-tag:${String(video.id)}`} videoId={video.id} />
              )}
              {owner && (
                // 題名 → タグ → 公開の順（ui-design.md「Visibility toggle」）。
                // 別の動画へ移ったら失敗の行を持ち越さないよう、id で作り直す。
                <VisibilitySwitch
                  key={`visibility:${String(video.id)}`}
                  videoId={video.id}
                  isPublic={video.public}
                  onChanged={() => void refresh()}
                />
              )}
            </div>
            {/* ゲストの応答には location が無いので、開く・コピーの操作は出ない。 */}
            <VideoFacts
              // 別の動画へ移ったら、送信中の指定と失敗の行を持ち越さない。
              key={`facts:${String(video.id)}`}
              video={video}
              owner={owner}
              capture={capture}
              onChanged={(saved, mark) => {
                replace(saved, mark);
                // グループの並びにあるこの動画のサムネイルも、画面のサムネイルとそろえる。
                rethumbRelated(saved);
              }}
              onStale={() => void refresh()}
              // 付け外しの後は動画を取り直し、取り直した `favorite` で塗りを確かめる
              // （specs/035-favorites/ui-design.md「Video page」）。
              onFavorite={setFavorite}
              versions={{
                navigation: {
                  backTo,
                  autoplay: status.playing || status.ended,
                },
                onReplace: replace,
                onRefresh: () => void refresh(),
              }}
            />
          </>
        )}
      </DetailPage>
    </div>
  );
}
