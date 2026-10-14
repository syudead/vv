import { createContext, useContext } from "react";

/**
 * GroupChanged は「Add to group」と「New group」が書き終えたことを、引き金の置かれた一覧に
 * 伝える。`targetId` は足した先のグループで、新しく作ったときは null である
 * （specs/048-groups/ui-design.md「Add to group」の Result）。一覧はそれで見せるものが変わる
 * ときだけ読み直す。ライブラリは足したものがいちばん上から外れるので読み直し、グループの
 * ページは足した先が自分のときだけ読み直す。
 */
export type GroupChanged = (targetId: number | null) => void;

function ignore(): void {
  // 一覧の外（動画ページなど）では何もしない。
}

/** GroupChangedContext は一覧（ライブラリ、グループのページ）が値を与える。 */
export const GroupChangedContext = createContext<GroupChanged>(ignore);

/** useGroupChanged は今の一覧の GroupChanged を返す。 */
export function useGroupChanged(): GroupChanged {
  return useContext(GroupChangedContext);
}
