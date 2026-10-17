import { useLocation, useNavigate } from "react-router";

import {
  groupContextOf,
  searchWithoutGroup,
  videoInGroupPath,
} from "../groups/groupPaths";
import {
  smartFolderContextOf,
  videoInSmartFolderPath,
} from "../smartFolders/smartFolderPaths";
import type { MemberTarget } from "./RelatedVideos";

/**
 * VideoGroupContext は、動画ページを開いたグループ（`?group`）かスマートフォルダ（`?smartFolder`）に
 * 従う場所と操作である。
 */
export type VideoGroupContext = {
  /** 読み取りに渡すグループの id。`?group` が無いか、プレイリストの文脈の間は undefined。 */
  group: number | undefined;
  /**
   * 読み取りに渡すスマートフォルダの id。`?smartFolder` が無いか、`?group` があれば undefined
   * （`group` が勝つ）。プレイリストの文脈の間も読む。フォルダの項目の中で、フォルダの行と
   * フォルダの `Up next` を出すためである。
   */
  smartFolder: number | undefined;
  /** この画面から別の動画を開く場所。`?group` か `?smartFolder` があれば残す。 */
  videoTo: (videoId: number) => string;
  /** 次の動画を開く場所。次が無ければ undefined。 */
  nextTo: (next: { id: number } | undefined) => string | undefined;
  /**
   * `?group` か `?smartFolder` で開いた動画のメンバーの行の開き方。どちらも無いか、プレイリストの
   * 文脈の間は undefined（ふつうの行か、ページが決める行）。
   */
  memberTarget: (linkState: unknown) => MemberTarget | undefined;
  /**
   * グループの操作（外す・タグ化・解散）の後に、動画と関連動画を `group` なしで読み直す。
   * `?group` があれば URL から外し（読み取りは問い合わせに従って読み直す）、無ければ `reread`
   * でそのまま取り直す。どちらも再生は止めない。
   */
  readWithoutGroup: (reread: () => void) => void;
};

/** contextVideoPath は、開いた文脈を保って動画 `videoId` を開く場所である。 */
function contextVideoPath(
  videoId: number,
  group: number | undefined,
  smartFolder: number | undefined,
): string {
  if (group !== undefined) return videoInGroupPath(videoId, group);
  if (smartFolder !== undefined) return videoInSmartFolderPath(videoId, smartFolder);
  return `/videos/${String(videoId)}`;
}

/**
 * useVideoGroupContext は動画ページの `?group` と `?smartFolder` を読む。プレイリストの文脈
 * （`inPlaylist`）はグループより優先するので、その間は `?group` を使わず、運ばない。
 * `?smartFolder` はその間も読み取りに渡すが、開く場所はプレイリストのステップが決める。
 */
export function useVideoGroupContext(inPlaylist: boolean): VideoGroupContext {
  const location = useLocation();
  const navigate = useNavigate();
  const group = inPlaylist ? undefined : groupContextOf(location.search);
  const smartFolder =
    group === undefined ? smartFolderContextOf(location.search) : undefined;
  const videoTo = (videoId: number) =>
    contextVideoPath(videoId, group, inPlaylist ? undefined : smartFolder);
  const opened = !inPlaylist && (group !== undefined || smartFolder !== undefined);
  return {
    group,
    smartFolder,
    videoTo,
    nextTo: (next) => (next === undefined ? undefined : videoTo(next.id)),
    memberTarget: (linkState) =>
      opened ? (memberId) => ({ to: videoTo(memberId), state: linkState }) : undefined,
    readWithoutGroup: (reread) => {
      if (group === undefined) {
        reread();
        return;
      }
      void navigate(
        { pathname: location.pathname, search: searchWithoutGroup(location.search) },
        { replace: true, state: location.state as unknown },
      );
    },
  };
}
