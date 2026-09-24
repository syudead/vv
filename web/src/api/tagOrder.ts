import type { TagRef } from "./client";

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
 *
 * - `add`: 既に付いていた（同じ id の）行は、サーバーが返した最新の name で
 *   差し替える（改名やシノニムからの付与直後に古い表示名が残らないように
 *   する）。無ければ名前の自然順を保つ位置へ挿す。
 * - `remove`: 同じ id の行を取り除く。無ければ変えない。
 */
export function applyTagToTags(
  tags: readonly TagRef[],
  tag: TagRef,
  action: "add" | "remove",
): TagRef[] {
  const withoutExisting = tags.filter((existing) => existing.id !== tag.id);
  if (action === "remove") return withoutExisting;
  // 既に付いていた同じ id の行も、いったん外してから挿し直す。改名やシノニムから
  // の付与で名前が変わっていれば、その最新の名前が並びにも反映される。
  const insertAt = withoutExisting.findIndex(
    (existing) => compareTagRefs(existing, tag) > 0,
  );
  const at = insertAt === -1 ? withoutExisting.length : insertAt;
  return [...withoutExisting.slice(0, at), tag, ...withoutExisting.slice(at)];
}
