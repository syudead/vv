import { useEffect, useRef } from "react";
import { useNavigate } from "react-router";

import type { PlaylistPlayback, PlaylistStep } from "../api/playlists";
import { t } from "../i18n";
import { itemTitle } from "../playlists/playlistItems";
import {
  type PlaylistQuery,
  playlistStepPath,
  stepPath,
} from "../playlists/playlistPlayback";
import { usePlaylistContext } from "../playlists/usePlaylistContext";
import { useToast } from "../ui/Toast";
import type { Neighbor } from "./NeighborArrows";
import type { VideoLinkState } from "./pageDecisions";
import type { MemberTarget } from "./RelatedVideos";

/**
 * useVideoPlaylist は動画ページのプレイリストの文脈（`?playlist=&item=`）である。所有者だけが
 * 読む（specs/044-playlists/ui-design.md「Video page in a playlist」、research.md R-8）。
 *
 * 文脈がある間は、矢印・予告・終わりが再生の応答に従い、関連動画の nextId・prevId は
 * 使わない（R-7）。404 で落としたら、前後は関連動画に戻り、今の動画の後で止まる。
 * `readKey` はページが再生の応答を読み直すたびに増やす値で、`ended` の間に読み終えた値と
 * 違えば読み直しの途中である（`reading`）。矢印とメンバーの行で移った先では、
 * `keepPlaying` なら再生を続ける。
 */
export function useVideoPlaylist({
  query,
  videoId,
  owner,
  readKey,
  ended,
  keepPlaying,
  linkState,
  smartFolder,
  adoptedSmartFolder,
}: {
  query: PlaylistQuery | null;
  videoId: number;
  owner: boolean;
  readKey: number;
  ended: boolean;
  keepPlaying: boolean;
  linkState: VideoLinkState;
  /** ページの `?smartFolder`。フォルダの項目の中でメンバーの行が保つ。 */
  smartFolder: number | undefined;
  /**
   * 動画の応答が採ったフォルダ（`Video.smartFolder` の id）。フォルダが動画を持たなくなると
   * サーバーは `smartFolder` を無視して答えるので、undefined になる。
   */
  adoptedSmartFolder: number | undefined;
}) {
  const navigate = useNavigate();
  const context = usePlaylistContext({ query, videoId, owner, readKey });
  const playlistId = query?.playlistId ?? 0;
  const itemId = query?.itemId ?? 0;
  const playback = context.playback;
  const inPlaylist =
    context.status === "reading" ||
    context.status === "ready" ||
    context.status === "failed";
  // ステップのフォルダは再生の応答の `smartFolderId` から取る（パネルの応答には頼らない。
  // specs/052-smart-folders/ui-design.md「Video page in a playlist」）。
  const goStep = (step: PlaylistStep, autoplay: boolean) =>
    void navigate(playlistStepPath(playlistId, step), {
      state: { ...linkState, autoplay },
    });
  const next = playback?.next;
  const stepNeighbor = (step: PlaylistStep | undefined): Neighbor | undefined =>
    step === undefined
      ? undefined
      : { title: step.video.title, go: () => goStep(step, keepPlaying) };
  const currentItem =
    context.panel.kind === "ready"
      ? context.panel.playlist.items.find((item) => item.id === itemId)
      : undefined;
  // グループとスマートフォルダの項目の中では、メンバー一覧の行もプレイリストの文脈を保ち、同じ
  // 項目のメンバーを開く（state はパネルの行と同じ）。項目がそうかは、パネルの並びか、次が同じ
  // 項目の中にあることで分かる。フォルダの項目では、動画の応答がページの `smartFolder` を採った
  // ときだけで、行は今のページの `smartFolder` を保つ。今の動画がフォルダから外れると、サーバーは
  // フォルダを無視して動画のグループで答え、その行は項目の外なので今までどおり開く
  // （specs/052-smart-folders/ui-design.md「Playlists」の States）。単体の動画の項目でも今までどおり。
  const folderItem = currentItem?.kind === "smartFolder" || smartFolder !== undefined;
  const inGroupItem =
    playback !== undefined &&
    (folderItem
      ? smartFolder !== undefined && adoptedSmartFolder === smartFolder
      : currentItem?.kind === "group" || playback.next?.itemId === playback.itemId);
  const memberTarget: MemberTarget | undefined =
    context.status === "ready" && playback !== undefined && inGroupItem
      ? (memberId) => ({
          to: stepPath(playlistId, memberId, playback.itemId, smartFolder),
          state: { ...linkState, autoplay: keepPlaying },
        })
      : undefined;

  return {
    context,
    playlistId,
    itemId,
    playback,
    /** 文脈を読んでいるか、読めたか、読み直しに失敗した。 */
    inPlaylist,
    /** 404 で文脈を落とした。 */
    dropped: context.status === "dropped",
    /** 終わった後の読み直しを待っている。その間は予告も終わりの層も出さない。 */
    reading: inPlaylist && ended && context.settledKey !== readKey,
    /** 再生の応答の次のステップ。 */
    next,
    /** 次のステップを自動再生で開く。 */
    playNext: () => {
      if (next !== undefined) goStep(next, true);
    },
    previousArrow: stepNeighbor(playback?.previousItem),
    nextArrow: stepNeighbor(playback?.nextItem),
    currentItem,
    memberTarget,
  };
}

/**
 * panelCurrentItemId はパネルで再生中にする項目である。再生の応答が無ければ URL の項目
 * `itemId`、位置が 0（項目がもう無い）なら無し。
 */
export function panelCurrentItemId(
  playback: PlaylistPlayback | undefined,
  itemId: number,
): number | undefined {
  if (playback === undefined) return itemId;
  return playback.position > 0 ? playback.itemId : undefined;
}

/** VideoPlaylist は useVideoPlaylist の返す文脈である。 */
export type VideoPlaylist = ReturnType<typeof useVideoPlaylist>;

/**
 * useUnplayableSkip は再生できない項目を飛ばす（ui-design.md「Unplayable item」）。再生の
 * できない状態（読み取りの失敗・再生できない・動画が無い）は終わらないので、自動再生で
 * 着いたときはページが自分で次の項目へ飛ばす。飛ばしたのは今の訪問（`pageKey`）の間だけ
 * 覚える。別の項目へ移れば忘れ、自動再生で同じ項目に戻ったときにもう一度飛ばす。
 */
export function useUnplayableSkip({
  playlist,
  autoplay,
  unplayable,
  videoTitle,
  pageKey,
  linkState,
}: {
  playlist: VideoPlaylist;
  autoplay: boolean;
  unplayable: boolean;
  /** 動画の題名。読めていなければ undefined で、項目の名前を知らせる。 */
  videoTitle: string | undefined;
  pageKey: string;
  linkState: VideoLinkState;
}) {
  const navigate = useNavigate();
  const toast = useToast();
  const { context, playback, playlistId, currentItem } = playlist;
  const skippedTitle =
    videoTitle ?? (currentItem === undefined ? "" : itemTitle(currentItem));
  const skippedFor = useRef<string | null>(null);
  // 次の効果より先に走る。
  useEffect(() => {
    skippedFor.current = null;
  }, [pageKey]);
  useEffect(() => {
    if (context.status !== "ready" || playback === undefined) return;
    if (!autoplay || !unplayable) return;
    if (skippedFor.current === pageKey) return;
    skippedFor.current = pageKey;
    const next = playback.next;
    if (next === undefined) {
      toast(t.player.playlist.skippedLast(skippedTitle));
      return;
    }
    toast(t.player.playlist.skipped(skippedTitle));
    void navigate(playlistStepPath(playlistId, next), {
      replace: true,
      state: { ...linkState, autoplay: true },
    });
  }, [
    autoplay,
    context.status,
    linkState,
    navigate,
    pageKey,
    playback,
    playlistId,
    skippedTitle,
    toast,
    unplayable,
  ]);
}
