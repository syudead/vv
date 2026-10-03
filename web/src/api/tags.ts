import { apiFetch, RequestFailed, request, toRequestFailed } from "./client";
import { clearListSnapshot } from "./listSnapshot";
import { nextVideoTagsSequence, recordAppliedVideoTags } from "./videoTagsEvents";
import type { components } from "./gen/openapi";

// 型は api/openapi.yaml からの生成物を使う（specs/014-video-tags/contracts/tags-api.md §1）。
export type Tag = components["schemas"]["Tag"];
export type TagRef = components["schemas"]["TagRef"];
export type VideoTagsResponse = components["schemas"]["VideoTagsResponse"];
export type VideoTagsSummary = components["schemas"]["VideoTagsSummary"];
export type RejectedTagNameList = components["schemas"]["RejectedTagNameList"];
export type TagBatchAction = components["schemas"]["TagBatchRequest"]["action"];
export type TagBatchResponse = components["schemas"]["TagBatchResponse"];
export type TagImpactAction = components["schemas"]["TagImpactRequest"]["action"];
export type TagImpactResponse = components["schemas"]["TagImpactResponse"];
export type TagMergeResponse = components["schemas"]["TagMergeResponse"];

/**
 * maxVideoTagsSelection は `POST /api/video-tags` の `videoIds` に許される上限
 * （contracts/tags-api.md §4）。この要求は全部か無しか（1件でも上限を超えると
 * 400 になり、何も変わらない）なので、選択がこれを超えるときは画面側で
 * 静かに分割して送らず、選択バーの一括操作を disabled のままにする
 * （web/src/library/SelectionBar.tsx、docs/design-docs/library-ui.md §6）。
 */
export const maxVideoTagsSelection = 20000;

/**
 * maxTagBatch は `POST /api/tags/batch`・`POST /api/tags/impact` の `ids` に許される上限
 * （specs/036-tag-admin-scale/contracts/screen-api.md §4）。超えると 400 `too_many_tags` に
 * なるので、画面は見えている行がこれを超えるときまとめての操作を disabled にする。
 */
export const maxTagBatch = 20000;

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
 * タグの一覧の共有の保持。
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
/**
 * latestFetch は、今の generation を持つ取得（進行中、または直近に held へ
 * 反映した取得）の Promise を指す。追い越された取得（下の startFetch の
 * stale 分岐）が「自分より新しい、今まさに勝っている取得」の結果を待つのに使う
 * （Devin の指摘: 追い越された取得は自分の――まだ変更を映していない――
 * 応答をそのまま返してはならない）。
 */
let latestFetch: Promise<Tag[]> | undefined;
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
 *
 * 追い越された取得は、自分の（まだその後の変更を映していない）応答を
 * 呼び出し元へそのまま返さない。`held` がまだ一度も埋まっていない最初の
 * 取得の間は `held` が無いので、それで代えると古い応答がそのまま漏れて
 * しまう（Devin の指摘）。代わりに、そのとき勝っている最新の取得
 * （`latestFetch`）の結果を待って、それを返す。勝っている取得自身がさらに
 * 追い越されていれば、そちらも同じように次を待つので、最終的には常に
 * 「実際に held を更新した取得」の結果に行き着く。
 */
function startFetch(): Promise<Tag[]> {
  generation += 1;
  const myGeneration = generation;
  const fetchPromise: Promise<Tag[]> = listTags().then((tags): Tag[] | Promise<Tag[]> => {
    if (myGeneration !== generation) {
      // startFetch は generation を上げた直後に必ず latestFetch を自分の
      // Promise へ差し替えるので、ここに来た時点で latestFetch は必ず
      // 埋まっている（自分より後の呼び出しが少なくとも1つある）。
      return latestFetch!;
    }
    held = tags;
    notify(tags);
    return tags;
  });
  latestFetch = fetchPromise;
  return fetchPromise;
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
  latestFetch = undefined;
  pendingGet = undefined;
  listeners.clear();
}

/**
 * refreshOnStaleTagError は、古いタグを使った操作の誤り（`tag_not_found`・
 * `tag_merge_required`・`tag_not_tentative`）を受けたときに共有の一覧を取り直す（別のタブでの
 * 削除・改名・統合・確定に、その場で気付けるようにする。specs/031-tentative-tags/
 * contracts/screen-api.md §2）。取り直しの完了は待たず、失敗しても揉み消して
 * 未処理の reject を残さない（B2）。誤りは常にそのまま投げ直す。
 */
function refreshOnStaleTagError(error: unknown): never {
  if (
    error instanceof RequestFailed &&
    (error.code === "tag_not_found" ||
      error.code === "tag_merge_required" ||
      error.code === "tag_not_tentative")
  ) {
    refreshTags().catch(() => undefined);
  }
  throw error;
}

/**
 * afterTagCreated は、タグを新しく作る操作が成功した後に呼ぶ。作ったタグは
 * 同じ名前の祖先フォルダの下にある既存の動画にもすぐ付く（017 の
 * data-model.md §4）ので、改名などと同じく動画一覧の控えを捨て、共有の
 * タグの一覧も取り直す（Structural Decisions 7・8）。
 */
function afterTagCreated(): void {
  afterTagChanged();
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

/**
 * refreshTagCounts は、付け外しで本数（`videoCount`）が変わったかもしれない
 * ときに呼ぶ。動画一覧やその控えには影響しないので `clearListSnapshot` は
 * 呼ばない。完了は待たず、失敗しても揉み消す（B2）。
 */
function refreshTagCounts(): void {
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
  const response = await apiFetch(`/api/tags/${String(id)}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    refreshOnStaleTagError(await toRequestFailed(response));
  }
  afterTagChanged();
}

/**
 * mergeTag は sourceIds のタグを id のタグへ統合する（1 件の統合も `[sourceId]` で送る）。
 * 返るのは統合先の更新後のタグ（`tag`）と、もう無かった統合元（`notFoundIds`）である
 * （specs/036-tag-admin-scale/contracts/screen-api.md §2・§4）。統合元がすべて無かったときも
 * 成功で、`tag` は変わらない統合先になる。
 */
export async function mergeTag(
  id: number,
  sourceIds: readonly number[],
  signal?: AbortSignal,
): Promise<TagMergeResponse> {
  const merged = await request<TagMergeResponse>(`/api/tags/${String(id)}/merge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sourceIds }),
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
  const response = await apiFetch(
    `/api/tags/${String(id)}/synonyms?${query.toString()}`,
    {
      method: "DELETE",
      signal,
    },
  );
  if (!response.ok) {
    refreshOnStaleTagError(await toRequestFailed(response));
  }
  afterTagChanged();
}

/**
 * confirmTag は仮のタグを確定する（POST /api/tags/{id}/confirm、
 * specs/031-tentative-tags/contracts/screen-api.md §2）。既に確定していても今の状態が返る。
 */
export async function confirmTag(id: number, signal?: AbortSignal): Promise<Tag> {
  const confirmed = await request<Tag>(`/api/tags/${String(id)}/confirm`, {
    method: "POST",
    signal,
  }).catch(refreshOnStaleTagError);
  afterTagChanged();
  return confirmed;
}

/**
 * rejectTag は仮のタグを却下する（POST /api/tags/{id}/reject、contracts/screen-api.md §2）。
 * タグは消え、付いていた動画から外れ、名前が却下した名前の一覧に入る。確定したタグは
 * 409 `tag_not_tentative` になる。却下した名前の一覧は呼び出し側が
 * `listRejectedTagNamePage` で取り直す。
 */
export async function rejectTag(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/tags/${String(id)}/reject`, {
    method: "POST",
    signal,
  });
  if (!response.ok) {
    refreshOnStaleTagError(await toRequestFailed(response));
  }
  afterTagChanged();
}

/**
 * batchTags は複数のタグをまとめて確定・却下・削除する（POST /api/tags/batch、
 * specs/036-tag-admin-scale/contracts/screen-api.md §1）。働かない種類・無いタグは
 * サーバーが飛ばし、`notApplicableIds`・`notFoundIds` で返す。成功したら、1 件の操作と同じく
 * `afterTagChanged` を 1 回呼ぶ。却下のあとの却下した名前の一覧は呼び出し側が取り直す。
 */
export async function batchTags(
  action: TagBatchAction,
  ids: readonly number[],
  signal?: AbortSignal,
): Promise<TagBatchResponse> {
  const result = await request<TagBatchResponse>("/api/tags/batch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action, ids: Array.from(ids) }),
    signal,
  });
  afterTagChanged();
  return result;
}

/**
 * tagImpact はまとめての却下・削除・統合の確認に出す、働くタグの数と影響を受ける動画の
 * 本数（重複なし）を返す（POST /api/tags/impact、contracts/screen-api.md §3）。何も変えない
 * ので、共有の保持には触れない。
 */
export function tagImpact(
  action: TagImpactAction,
  ids: readonly number[],
  signal?: AbortSignal,
): Promise<TagImpactResponse> {
  return request<TagImpactResponse>("/api/tags/impact", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action, ids: Array.from(ids) }),
    signal,
  });
}

/**
 * listRejectedTagNamePage は却下した名前の 1 ページを名前の自然順で返す
 * （GET /api/tags/rejected-names、specs/036-tag-admin-scale/contracts/screen-api.md §6）。
 * `cursor` は前のページの `nextCursor`、`limit` は 1 ページの件数（1〜200）。省くとサーバーの
 * 既定（先頭から 100 件）になる。応答の `total` は却下した名前の全部の数で、`nextCursor` は
 * 続きがあるときだけ入る。共有の保持には触れない。
 */
export function listRejectedTagNamePage(
  cursor?: string,
  limit?: number,
  signal?: AbortSignal,
): Promise<RejectedTagNameList> {
  const query = new URLSearchParams();
  if (cursor !== undefined) query.set("cursor", cursor);
  if (limit !== undefined) query.set("limit", String(limit));
  const search = query.toString();
  return request<RejectedTagNameList>(
    search === "" ? "/api/tags/rejected-names" : `/api/tags/rejected-names?${search}`,
    { signal },
  );
}

/**
 * forgetRejectedTagName は名前を却下した名前の一覧から外す
 * （DELETE /api/tags/rejected-names?name=…、contracts/screen-api.md §3）。一覧に無い名前でも
 * 成功する。外した名前は、次に自動で付けたとき再び仮のタグとして作られる。
 */
export async function forgetRejectedTagName(
  name: string,
  signal?: AbortSignal,
): Promise<void> {
  const query = new URLSearchParams({ name });
  const response = await apiFetch(`/api/tags/rejected-names?${query.toString()}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

/**
 * updateVideoTags は動画へタグを付ける・外す（POST /api/video-tags、
 * contracts/tags-api.md §4）。成功したら、読み込み済みの一覧の項目と
 * listSnapshot の控えの tags へ結果を反映する通知を送る（Structural
 * Decisions 7）。`tag_not_found` を受けたときは共有のタグの一覧も取り直す。
 */
async function updateVideoTags(
  videoIds: readonly number[],
  action: "add" | "remove",
  tag: { id: number } | { name: string },
  signal?: AbortSignal,
): Promise<VideoTagsResponse> {
  // 通し番号は送信の直前に払い出す。応答が届く順は送った順と限らないので、
  // 反映するときにこの番号で古い応答を捨てる（N1）。
  const sequence = nextVideoTagsSequence();
  const result = await request<VideoTagsResponse>("/api/video-tags", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ videoIds: Array.from(videoIds), action, tag }),
    signal,
  }).catch(refreshOnStaleTagError);
  recordAppliedVideoTags(videoIds, result.tag, action, sequence);
  return result;
}

/**
 * attachVideoTagByID は id で指定したタグを videoIds の動画へ付ける。
 * 再生画面の1本も、選択バーの複数本も、これを使う。本数が変わるので、
 * 共有のタグの一覧も取り直す（自己レビュー B4／(5)。候補の本数を新しく保つ）。
 */
export function attachVideoTagByID(
  videoIds: readonly number[],
  tagId: number,
  signal?: AbortSignal,
): Promise<VideoTagsResponse> {
  return updateVideoTags(videoIds, "add", { id: tagId }, signal).then((result) => {
    refreshTagCounts();
    return result;
  });
}

/**
 * attachVideoTagByName は名前でタグを付ける。名前はシノニムを含めて引き、
 * 無ければ作る（contracts/tags-api.md §4）。新しいタグが作られているかも
 * しれないので、共有のタグの一覧を取り直す
 * （issue 268 の受け入れ条件1「別の動画の再生画面でタグを追加しようとすると、
 * そのタグが候補に出る」）。送る前の共有の一覧に無いタグが返ったときは
 * 作られたとみなし、`createTag` と同じく動画一覧の控えも捨てる（作った
 * タグはフォルダ名から既存の動画にも付くため。017 の data-model.md §4）。
 */
export function attachVideoTagByName(
  videoIds: readonly number[],
  name: string,
  signal?: AbortSignal,
): Promise<VideoTagsResponse> {
  const known = currentTags();
  return updateVideoTags(videoIds, "add", { name }, signal).then((result) => {
    if (known?.some((tag) => tag.id === result.tag.id)) {
      refreshTagCounts();
    } else {
      afterTagCreated();
    }
    return result;
  });
}

/**
 * detachVideoTag は id で指定したタグを videoIds の動画から外す。外すときは
 * 常に id で指定する（画面が外す候補はいつも付いているタグで、id を
 * 持っているため。contracts/tags-api.md §4）。本数が変わるので、共有の
 * タグの一覧も取り直す。
 */
export function detachVideoTag(
  videoIds: readonly number[],
  tagId: number,
  signal?: AbortSignal,
): Promise<VideoTagsResponse> {
  return updateVideoTags(videoIds, "remove", { id: tagId }, signal).then((result) => {
    refreshTagCounts();
    return result;
  });
}

/**
 * summarizeVideoTags は選んだ動画に付いたタグの要約を返す
 * （POST /api/video-tags/summary、contracts/tags-api.md §4）。
 */
export function summarizeVideoTags(
  videoIds: readonly number[],
  signal?: AbortSignal,
): Promise<VideoTagsSummary> {
  return request<VideoTagsSummary>("/api/video-tags/summary", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ videoIds: Array.from(videoIds) }),
    signal,
  });
}
