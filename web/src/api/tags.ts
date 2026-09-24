import { RequestFailed, request, toRequestFailed } from "./client";
import { clearListSnapshot } from "./listSnapshot";
import type { components } from "./gen/openapi";

// 型は api/openapi.yaml からの生成物を使う（specs/014-video-tags/contracts/tags-api.md §1）。
export type Tag = components["schemas"]["Tag"];

/**
 * listTags はタグを名前の自然順で取得する（本数0を含む）。
 *
 * 呼び出し元ごとの AbortSignal は受け取らない。この要求は複数の呼び出し元で
 * 共有するので（下の共有の保持）、1人が打ち切っても他の待ち手を巻き込んで
 * はならない（B2）。
 */
function listTags(): Promise<Tag[]> {
  return request<{ items: Tag[] }>("/api/tags").then((page) => page.items);
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
/** generation は最後に始めた取得の通し番号。取得のたびに増える。 */
let generation = 0;
/** pendingGet は getTags 同士（held がまだ無いときの同時呼び出し）だけをまとめる。 */
let pendingGet: Promise<Tag[]> | undefined;
const listeners = new Set<TagsListener>();

function notify(tags: Tag[]): void {
  for (const listener of listeners) listener(tags);
}

/**
 * startFetch は新しい取得を1つ必ず始める。呼んだ時点の generation より後で
 * 別の取得が始まっていれば（応答が届く順にかかわらず）、この結果は古いので
 * `held` に反映しない（B1）。取り直しのたびに独立した取得になるので、先に
 * 始まっていた取得（進行中の GET）を巻き込む・巻き込まれることもない（B1・B2）。
 */
function startFetch(): Promise<Tag[]> {
  generation += 1;
  const myGeneration = generation;
  return listTags().then((tags) => {
    if (myGeneration !== generation) return held ?? tags;
    held = tags;
    notify(tags);
    return tags;
  });
}

/**
 * getTags は共有の保持を返す。すでに取得済みならそのまま返し、まだなければ
 * `GET /api/tags` を送る。held が無い間に複数箇所から呼ばれても、その分だけは
 * 同じ1つの要求にまとめる（`refreshTags` の取り直しはまとめない。B1）。
 */
export function getTags(): Promise<Tag[]> {
  if (held !== undefined) return Promise.resolve(held);
  if (pendingGet === undefined) {
    pendingGet = startFetch().finally(() => {
      pendingGet = undefined;
    });
  }
  return pendingGet;
}

/** currentTags はまだ取得していなければ undefined を返す、同期の読み出しである。 */
export function currentTags(): Tag[] | undefined {
  return held;
}

/**
 * refreshTags はサーバーから必ず新しく取り直し、共有の保持を差し替えて購読者に
 * 知らせる。進行中の（`getTags` などによる）取得があっても、それには乗らず
 * 別の要求を送る（B1）。
 */
export function refreshTags(): Promise<Tag[]> {
  return startFetch();
}

/** subscribeTags は共有の保持が取り直されるたびに呼ばれる。戻り値で購読をやめる。 */
export function subscribeTags(listener: TagsListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * __resetTagsForTest はテストだけで使う。モジュールは1つのタブに1つの共有の
 * 保持を持つ前提で、通常は画面が終わるまでリセットしない。Vitest はテスト
 * ファイル単位でモジュールを使い回すので、テストどうしで保持が残らないように
 * ここでだけ明示的に空へ戻す（N5）。
 */
export function __resetTagsForTest(): void {
  held = undefined;
  generation = 0;
  pendingGet = undefined;
  listeners.clear();
}

/**
 * refreshOnStaleTagError は、古いタグを使った操作の誤り（`tag_not_found`・
 * `tag_merge_required`）を受けたときに共有の一覧を取り直す（親 Issue の
 * 子 Issue「タグの管理 API を公開する」項目5、Plan の Structural Decisions 7・8
 * — 別のタブでの削除・改名・統合に、
 * その場で気付けるようにする）。取り直しの完了は待たず、失敗しても揉み消して
 * 未処理の reject を残さない（B2）。誤りは常にそのまま投げ直す。
 */
function refreshOnStaleTagError(error: unknown): never {
  if (
    error instanceof RequestFailed &&
    (error.code === "tag_not_found" || error.code === "tag_merge_required")
  ) {
    refreshTags().catch(() => undefined);
  }
  throw error;
}

/**
 * afterTagCreated は、タグを新しく作る操作が成功した後に呼ぶ。共有のタグの
 * 一覧を取り直す（Structural Decisions 8）。作成は既存の動画の一覧に影響
 * しないので、`clearListSnapshot` は呼ばない（Structural Decisions 7 は改名・
 * 削除・統合・シノニムの変更だけを挙げている。N1）。
 */
function afterTagCreated(): void {
  refreshTags().catch(() => undefined);
}

/**
 * afterTagChanged は、既存のタグを書き換える操作（改名・削除・統合・シノニム
 * の変更）が成功した後に呼ぶ。動画一覧の控え（`listSnapshot`）は、破棄して
 * 読み直す（メディアフォルダの変更と同じ扱い。Structural Decisions 7）。共有の
 * タグの一覧も取り直す（Structural Decisions 8）。どちらも完了を待たなくてよい。
 */
function afterTagChanged(): void {
  clearListSnapshot();
  refreshTags().catch(() => undefined);
}

/** createTag はタグを1件作る。 */
export async function createTag(name: string, signal?: AbortSignal): Promise<Tag> {
  const created = await request<Tag>("/api/tags", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  }).catch(refreshOnStaleTagError);
  afterTagCreated();
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
  }).catch(refreshOnStaleTagError);
  afterTagChanged();
  return updated;
}

/** deleteTag はタグを1件削除する。 */
export async function deleteTag(id: number, signal?: AbortSignal): Promise<void> {
  const response = await fetch(`/api/tags/${String(id)}`, { method: "DELETE", signal });
  if (!response.ok) {
    refreshOnStaleTagError(await toRequestFailed(response));
  }
  afterTagChanged();
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
  }).catch(refreshOnStaleTagError);
  afterTagChanged();
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
  }).catch(refreshOnStaleTagError);
  afterTagChanged();
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
    refreshOnStaleTagError(await toRequestFailed(response));
  }
  afterTagChanged();
}
