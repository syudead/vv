import { request, toRequestFailed } from "./client";
import { clearListSnapshot } from "./listSnapshot";
import type { components } from "./gen/openapi";

// 型は api/openapi.yaml からの生成物を使う（specs/014-video-tags/contracts/tags-api.md §1）。
export type Tag = components["schemas"]["Tag"];

/** listTags はタグを名前の自然順で取得する（本数0を含む）。 */
function listTags(signal?: AbortSignal): Promise<Tag[]> {
  return request<{ items: Tag[] }>("/api/tags", { signal }).then((page) => page.items);
}

/**
 * タグの一覧の共有の保持（Plan の Structural Decisions 8）。
 *
 * 候補（combobox）・絞り込み中のタグの確かめ・管理画面が同じ控えを読む。画面ごとに
 * `GET /api/tags` を持つと、同じタブの中で「管理画面で作ったタグが開いたままの
 * 再生画面の候補に出ない」食い違いが起きる。**直近の1回分**だけを持ち、
 * 画面が開くとき・タグを変えたとき・`tag_not_found` か `missingTagIds` を受けたときに
 * `refreshTags` で取り直す（取り直しの呼び出し自体は、それぞれを扱う画面の単位で行う）。
 */
type TagsListener = (tags: Tag[]) => void;

let held: Tag[] | undefined;
let inFlight: Promise<Tag[]> | undefined;
const listeners = new Set<TagsListener>();

function notify(tags: Tag[]): void {
  for (const listener of listeners) listener(tags);
}

/**
 * getTags は共有の保持を返す。すでに取得済みならそのまま返し、まだなら
 * `GET /api/tags` を1回だけ送る（同時に呼ばれても要求は1つにまとめる）。
 */
export function getTags(signal?: AbortSignal): Promise<Tag[]> {
  if (held !== undefined) return Promise.resolve(held);
  return refreshTags(signal);
}

/** currentTags はまだ取得していなければ undefined を返す、同期の読み出しである。 */
export function currentTags(): Tag[] | undefined {
  return held;
}

/**
 * refreshTags はサーバーから取り直し、共有の保持を差し替えて購読者に知らせる。
 * 同時に呼ばれた分は同じ要求にまとめる。
 */
export function refreshTags(signal?: AbortSignal): Promise<Tag[]> {
  if (inFlight === undefined) {
    inFlight = listTags(signal).finally(() => {
      inFlight = undefined;
    });
  }
  return inFlight.then((tags) => {
    held = tags;
    notify(tags);
    return tags;
  });
}

/** subscribeTags は共有の保持が取り直されるたびに呼ばれる。戻り値で購読をやめる。 */
export function subscribeTags(listener: TagsListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * afterTagsChanged は、タグそのものを書き換える操作が成功した後に呼ぶ。
 *
 * 動画一覧の控え（`listSnapshot`）は、破棄して読み直す（メディアフォルダの変更と
 * 同じ扱い。Plan の Structural Decisions 7）。共有のタグの一覧も取り直す
 * （Structural Decisions 8）。呼び出し側はこの完了を待たなくてよい。
 */
function afterTagsChanged(): void {
  clearListSnapshot();
  void refreshTags();
}

/** createTag はタグを1件作る。 */
export async function createTag(name: string, signal?: AbortSignal): Promise<Tag> {
  const created = await request<Tag>("/api/tags", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  });
  afterTagsChanged();
  return created;
}

/** renameTag はタグの元の名前を書き換える。今と同じ名前なら何も変えずに今の状態が返る。 */
export async function renameTag(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<Tag> {
  const updated = await request<Tag>(`/api/tags/${String(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  });
  afterTagsChanged();
  return updated;
}

/** deleteTag はタグを1件削除する。 */
export async function deleteTag(id: number, signal?: AbortSignal): Promise<void> {
  const response = await fetch(`/api/tags/${String(id)}`, { method: "DELETE", signal });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
  afterTagsChanged();
}

/**
 * mergeTag は sourceId のタグを id のタグへ統合する。返るのは統合先の更新後の
 * タグである。
 */
export async function mergeTag(
  id: number,
  sourceId: number,
  signal?: AbortSignal,
): Promise<Tag> {
  const merged = await request<Tag>(`/api/tags/${String(id)}/merge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sourceId }),
    signal,
  });
  afterTagsChanged();
  return merged;
}

/**
 * addTagSynonym は名前を id のタグのシノニムにする。
 *
 * 名前が別のタグ S の元の名前で、確認をとって承諾したタグの id を `mergeTagId` に
 * 渡すと、S を id へ統合する（承諾していなければ渡さない）。承諾していないタグと
 * 違えば 409 `tag_merge_required` になる（contracts/tags-api.md §3）。
 */
export async function addTagSynonym(
  id: number,
  name: string,
  mergeTagId?: number,
  signal?: AbortSignal,
): Promise<Tag> {
  const updated = await request<Tag>(`/api/tags/${String(id)}/synonyms`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(mergeTagId === undefined ? { name } : { name, mergeTagId }),
    signal,
  });
  afterTagsChanged();
  return updated;
}

/** removeTagSynonym は name を id のタグのシノニムから外す。 */
export async function removeTagSynonym(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<void> {
  const query = new URLSearchParams({ name });
  const response = await fetch(`/api/tags/${String(id)}/synonyms?${query.toString()}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
  afterTagsChanged();
}
