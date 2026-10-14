import { useLocation, useNavigate } from "react-router";

import {
  groupContextOf,
  searchWithoutGroup,
  videoInGroupPath,
} from "../groups/groupPaths";
import type { MemberTarget } from "./RelatedVideos";

/**
 * VideoGroupContext は、動画ページを開いたグループ（`?group`）に従う場所と操作である
 * （specs/048-groups/ui-design.md「Video page」、research.md R-9）。
 */
export type VideoGroupContext = {
  /** 読み取りに渡すグループの id。`?group` が無いか、プレイリストの文脈の間は undefined。 */
  group: number | undefined;
  /** この画面から別の動画を開く場所。`?group` があれば残す。 */
  videoTo: (videoId: number) => string;
  /** 次の動画を開く場所。次が無ければ undefined。 */
  nextTo: (next: { id: number } | undefined) => string | undefined;
  /** `?group` で開いた動画のメンバーの行の開き方。`?group` が無ければ undefined（ふつうの行）。 */
  memberTarget: (linkState: unknown) => MemberTarget | undefined;
  /**
   * グループの操作（外す・タグ化・解散）の後に、動画と関連動画を `group` なしで読み直す。
   * `?group` があれば URL から外し（読み取りは問い合わせに従って読み直す）、無ければ `reread`
   * でそのまま取り直す。どちらも再生は止めない。
   */
  readWithoutGroup: (reread: () => void) => void;
};

/**
 * useVideoGroupContext は動画ページの `?group` を読む。プレイリストの文脈（`inPlaylist`）は
 * グループより優先するので、その間は `?group` を使わず、運ばない。
 */
export function useVideoGroupContext(inPlaylist: boolean): VideoGroupContext {
  const location = useLocation();
  const navigate = useNavigate();
  const group = inPlaylist ? undefined : groupContextOf(location.search);
  const videoTo = (videoId: number) =>
    group === undefined ? `/videos/${String(videoId)}` : videoInGroupPath(videoId, group);
  return {
    group,
    videoTo,
    nextTo: (next) => (next === undefined ? undefined : videoTo(next.id)),
    memberTarget: (linkState) =>
      group === undefined
        ? undefined
        : (memberId) => ({ to: videoTo(memberId), state: linkState }),
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
