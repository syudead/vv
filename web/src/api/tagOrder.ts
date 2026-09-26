import type { TagRef, VideoTag } from "./client";

/**
 * タグの名前の自然順（specs/014-video-tags/contracts/tags-api.md §1、
 * ui-design.md「付け足したタグは名前順に挿す」）。
 *
 * `internal/domain.CompareNatural` を写す: 数字の連続は数値として比べ
 * （`2` < `10`）、それ以外は大文字小文字を区別せずに比べる。サロゲート
 * ペア（絵文字など）も1つの文字として扱うため、`Array.from` で符号位置単位に
 * 区切る（`string[index]` は UTF-16 コード単位単位になってしまう）。
 */
export function compareNatural(a: string, b: string): number {
  const ca = Array.from(a);
  const cb = Array.from(b);
  let i = 0;
  let j = 0;
  while (i < ca.length && j < cb.length) {
    const ra = ca[i] ?? "";
    const rb = cb[j] ?? "";
    if (isDigit(ra) && isDigit(rb)) {
      let da = "";
      while (i < ca.length && isDigit(ca[i] ?? "")) {
        da += ca[i];
        i += 1;
      }
      let db = "";
      while (j < cb.length && isDigit(cb[j] ?? "")) {
        db += cb[j];
        j += 1;
      }
      const order = compareDigitRun(da, db);
      if (order !== 0) return order;
      continue;
    }
    const la = ra.toLowerCase();
    const lb = rb.toLowerCase();
    if (la !== lb) return la < lb ? -1 : 1;
    i += 1;
    j += 1;
  }
  if (i >= ca.length && j >= cb.length) return 0;
  return i >= ca.length ? -1 : 1;
}

function isDigit(ch: string): boolean {
  return ch >= "0" && ch <= "9";
}

/** compareDigitRun は数字の連続を、桁あふれせずに数値として比べる。 */
function compareDigitRun(a: string, b: string): number {
  const ta = a.replace(/^0+/, "");
  const tb = b.replace(/^0+/, "");
  if (ta.length !== tb.length) return ta.length < tb.length ? -1 : 1;
  if (ta === tb) return 0;
  return ta < tb ? -1 : 1;
}

/**
 * compareTagRefs は `TagRef` を名前の自然順で比べる。同順位は id で決着させる
 * （`internal/domain.SortTagRefs` と同じ規則）。
 */
export function compareTagRefs(a: TagRef, b: TagRef): number {
  const byName = compareNatural(a.name, b.name);
  if (byName !== 0) return byName;
  if (a.name !== b.name) return a.name < b.name ? -1 : 1;
  return a.id - b.id;
}

/**
 * applyTagToTags は付け外しの結果を1件の `tags` 配列へ反映する。
 * listSnapshot.ts と useVideos.ts の両方がこれを使う（唯一の実装場所）。
 * 付け外しは手で付けた分（`manual`）だけを変え、フォルダ名から付いている分
 * （`fromFolder`）はそのまま残す（specs/017-folder-groups/contracts/folder-groups-api.md §4）。
 *
 * - `add`: 手で付けた分を立てる。既に付いていた（同じ id の）行は、サーバーが
 *   返した最新の name で差し替える（改名やシノニムからの付与直後に古い表示名が
 *   残らないようにする）。無ければ名前の自然順を保つ位置へ挿す。
 * - `remove`: 手で付けた分を外す。フォルダ名からも付いている行は残し、そうで
 *   なければ取り除く。無ければ変えない。
 */
export function applyTagToTags(
  tags: readonly VideoTag[],
  tag: TagRef,
  action: "add" | "remove",
): VideoTag[] {
  const existing = tags.find((candidate) => candidate.id === tag.id);
  const withoutExisting = tags.filter((candidate) => candidate.id !== tag.id);
  if (action === "remove") {
    if (existing?.fromFolder !== true) return withoutExisting;
    return tags.map((candidate) =>
      candidate.id === tag.id ? { ...candidate, manual: false } : candidate,
    );
  }
  const added: VideoTag = {
    id: tag.id,
    name: tag.name,
    manual: true,
    fromFolder: existing?.fromFolder ?? false,
  };
  // 既に付いていた同じ id の行も、いったん外してから挿し直す。改名やシノニムから
  // の付与で名前が変わっていれば、その最新の名前が並びにも反映される。
  const insertAt = withoutExisting.findIndex(
    (candidate) => compareTagRefs(candidate, added) > 0,
  );
  const at = insertAt === -1 ? withoutExisting.length : insertAt;
  return [...withoutExisting.slice(0, at), added, ...withoutExisting.slice(at)];
}

/**
 * tagsReflectChange は、動画の `tags` がすでに付け外しの結果を映しているかを
 * 返す。付け外しが変えるのは手で付けた分（`manual`）だけなので、同じ id の行が
 * あるかではなく、その行の `manual` で判断する。フォルダ名からだけ付いている行
 * （`manual: false`）は、手で付けた結果をまだ映していない。
 */
export function tagsReflectChange(
  tags: readonly VideoTag[],
  tagId: number,
  action: "add" | "remove",
): boolean {
  const manual = tags.some((tag) => tag.id === tagId && tag.manual);
  return manual === (action === "add");
}

/**
 * isFolderOnly は、フォルダ名からだけ付いている（手では付けていない）タグかを
 * 返す。この形だけを破線のチップにし、再生画面では × を出さない
 * （specs/017-folder-groups/ui-design.md「Folder-derived tag chip」）。
 */
export function isFolderOnly(tag: VideoTag): boolean {
  return tag.fromFolder && !tag.manual;
}
