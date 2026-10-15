import { maxBundleSelection } from "../api/client";
import type { LibraryGroup, Video } from "../api/client";
import { t, type UiText } from "../i18n";
import type { OntoAnswer } from "./cardDragRules";

// 動画を動画に落としたときの規則である（specs/050-drag-organize/ui-design.md「What a drop
// does」「Drop menus」「The new group's name」）。画面（ライブラリ、グループのページ、フォルダ
// 画面）が共有する。

/** groupNameLimit はグループの名前の長さの上限（コードポイント）である（domain.ValidateGroupName）。 */
export const groupNameLimit = 100;

/**
 * videoOnto は動画のカードの上に `lifted` を落とせるかを返す。動画だけなら「Group or bundle…」、
 * グループが混じれば拒否（グループからはグループを作らない。contracts/screen-api.md `/group`）。
 */
export function videoOnto(liftedAllVideos: boolean): OntoAnswer<UiText> {
  return liftedAllVideos ? { label: t.organize.drop.groupOrBundle } : "refused";
}

/**
 * bundleOverLimit は持ち上げたカード `liftedCount` 枚と落とした先の 1 枚が、束ねられる 20 本を
 * 超えるかを返す。画面はカードを数え、集まりの展開はサーバーに任せる（ui-design.md「Bundle limit」）。
 */
export function bundleOverLimit(liftedCount: number): boolean {
  return liftedCount + 1 > maxBundleSelection;
}

/** codePoints は文字列のコードポイントの数である。 */
function codePoints(value: string): number {
  return Array.from(value).length;
}

/**
 * whiteSpace は Unicode の White_Space（Go の unicode.White_Space）である。JS の `trim` が削る
 * 集合とは U+0085 と U+FEFF で食い違うので、サーバーと同じ集合を明示する。
 */
const whiteSpace =
  "\t\n\v\f\r \u0085\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000";
const leadingWhiteSpace = new RegExp(`^[${whiteSpace}]+`, "u");
const trailingWhiteSpace = new RegExp(`[${whiteSpace}]+$`, "u");

/**
 * provisionalGroupName はサーバーが「Make a group」の新しいグループに付ける名前である。落とした先の
 * 題名の先頭の空白を除き、先頭 100 コードポイントに切り、末尾の空白を除いたもの
 * （domain.GroupNameFromTitle、data-model.md「Operations」の Group）。順序もサーバーと同じにする。
 */
export function provisionalGroupName(title: string): string {
  const cut = Array.from(title.replace(leadingWhiteSpace, ""))
    .slice(0, groupNameLimit)
    .join("");
  return cut.replace(trailingWhiteSpace, "");
}

/** NameFieldOutcome は名前の欄で Enter を押した結果である。 */
export type NameFieldOutcome =
  { kind: "keep" } | { kind: "invalid" } | { kind: "rename"; name: string };

/**
 * nameFieldOutcome は名前の欄の値 `value` で Enter を押した結果を返す。前後を削って空か仮の名前と
 * 同じなら何も送らずに閉じ、100 コードポイントを超えれば閉じない（ui-design.md「The new group's
 * name」）。
 */
export function nameFieldOutcome(value: string, provisional: string): NameFieldOutcome {
  const name = value.trim();
  if (name === "" || name === provisional) return { kind: "keep" };
  if (codePoints(name) > groupNameLimit) return { kind: "invalid" };
  return { kind: "rename", name };
}

/** nameTooLong は名前の欄の値が長すぎるか（欄の下に規則を出すか）を返す。 */
export function nameTooLong(value: string): boolean {
  return codePoints(value.trim()) > groupNameLimit;
}

function latest(values: readonly string[], fallback: string): string {
  return values.reduce((max, value) => (value > max ? value : max), fallback);
}

/**
 * provisionalGroup は「Make a group」の結果をサーバーが答える前に見せる、新しいグループのカードの
 * 値である。落とした先 `target`、続いて `lifted` を画面の順に持つ。所有者の画面だけが使う。
 */
export function provisionalGroup(
  id: number,
  target: Video,
  lifted: readonly Video[],
): LibraryGroup {
  const members = [target, ...lifted];
  const durations = members.flatMap((video) =>
    video.durationMs === undefined ? [] : [video.durationMs],
  );
  return {
    id,
    name: provisionalGroupName(target.title),
    videoCount: members.length,
    watchedCount: 0,
    watchState: "unwatched",
    ...(durations.length === 0
      ? {}
      : { durationMs: durations.reduce((sum, value) => sum + value, 0) }),
    sizeBytes: members.reduce((sum, video) => sum + video.sizeBytes, 0),
    addedAt: latest(
      members.map((video) => video.addedAt),
      target.addedAt,
    ),
    previews: members
      .flatMap((video) =>
        video.thumbnailUrl === undefined
          ? []
          : [{ videoId: video.id, thumbnailUrl: video.thumbnailUrl }],
      )
      .slice(0, 4),
    openVideoId: target.id,
    videoIds: members.map((video) => video.id),
    tags: [],
    favorite: false,
  };
}

/**
 * DropOverlay はサーバーが答える前に見せる、落とした結果である（R-8）。`made` があれば落とした
 * 先がその項目（新しいグループ）になり、無ければ持ち上げたカードだけが消える（束ねた）。
 */
export interface DropOverlay<T> {
  target: string;
  lifted: ReadonlySet<string>;
  made: T | null;
}

/** applyDropOverlay は並び `items` に落とした結果 `overlay` を重ねた並びを返す。 */
export function applyDropOverlay<T>(
  items: readonly T[],
  keyOf: (item: T) => string,
  overlay: DropOverlay<T> | null,
): readonly T[] {
  if (overlay === null) return items;
  if (overlay.made === null)
    return items.filter((item) => !overlay.lifted.has(keyOf(item)));
  return replaceWithGroup(items, keyOf, overlay.target, overlay.lifted, overlay.made);
}

/**
 * replaceWithGroup は並び `items` のうち、落とした先 `target` を新しい項目 `made` に置き換え、
 * 持ち上げた項目 `lifted` を除いた並びを返す。新しいグループは落とした先の位置を取る（R-6）。
 */
export function replaceWithGroup<T>(
  items: readonly T[],
  keyOf: (item: T) => string,
  target: string,
  lifted: ReadonlySet<string>,
  made: T,
): T[] {
  return items.flatMap((item) => {
    const key = keyOf(item);
    if (key === target) return [made];
    return lifted.has(key) ? [] : [item];
  });
}
