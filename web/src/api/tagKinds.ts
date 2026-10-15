import { apiFetch, RequestFailed, request, toRequestFailed } from "./client";
import { clearListSnapshot } from "./listSnapshot";
import { refreshTags } from "./tags";
import type { components } from "./gen/openapi";

// タグの種類（specs/051-tag-kinds/contracts/screen-api.md の `web/src/api` functions）。型は
// api/openapi.yaml からの生成物を使う。
export type TagKind = components["schemas"]["TagKind"];
export type TagKindRef = components["schemas"]["TagKindRef"];
export type TagKindList = components["schemas"]["TagKindList"];

/**
 * 種類の一覧の共有の保持。`api/tags.ts` のタグの一覧と同じく、種類の管理・タグの種類の
 * メニュー・ライブラリの絞り込みが同じ控えを読む。**直近の 1 回分**だけを持ち、種類を
 * 変えたとき（`tag_kind_not_found` を受けたときを含む）に取り直すか捨てる。
 */
type TagKindsListener = (kinds: TagKind[]) => void;

let held: TagKind[] | undefined;
/** generation は最後に始めた取得、または保持を捨てた時点の通し番号である。 */
let generation = 0;
/** latestFetch は今の generation を持つ取得である。保持を捨てたあとは無い。 */
let latestFetch: Promise<TagKind[]> | undefined;
const listeners = new Set<TagKindsListener>();

/**
 * listTagKinds は種類をすべて取得する（`GET /api/tag-kinds`）。サーバーは id の順に返す。
 * 呼び出し元ごとの AbortSignal は受け取らない。この要求は複数の呼び出し元で共有する。
 */
export function listTagKinds(): Promise<TagKind[]> {
  return request<TagKindList>("/api/tag-kinds").then((list) => list.items);
}

/**
 * startFetch は新しい取得を 1 つ始める。届くまでに別の取得が始まったか保持が捨てられたら、
 * この応答は古いので `held` に入れず、そのとき勝っている取得（無ければ新しく始める取得）の
 * 結果を返す。種類を変えた後の呼び手に、変更の前の一覧を返さないためである。
 */
function startFetch(): Promise<TagKind[]> {
  generation += 1;
  const myGeneration = generation;
  const fetching: Promise<TagKind[]> = listTagKinds().then(
    (kinds): TagKind[] | Promise<TagKind[]> => {
      if (myGeneration !== generation) return latestFetch ?? startFetch();
      held = kinds;
      for (const listener of listeners) listener(kinds);
      return kinds;
    },
  );
  // 失敗した取得を待ち続けないように外し、次の `getTagKinds` に取り直させる。
  fetching.catch(() => {
    if (latestFetch === fetching) latestFetch = undefined;
  });
  latestFetch = fetching;
  return fetching;
}

/**
 * getTagKinds は共有の保持を返す。まだ無ければ `GET /api/tag-kinds` を送り、取得の途中なら
 * その結果を待つ。
 */
export function getTagKinds(): Promise<TagKind[]> {
  if (held !== undefined) return Promise.resolve(held);
  return latestFetch ?? startFetch();
}

/** currentTagKinds はまだ取得していなければ undefined を返す、同期の読み出しである。 */
export function currentTagKinds(): TagKind[] | undefined {
  return held;
}

/** refreshTagKinds はサーバーから必ず取り直し、共有の保持を差し替えて購読者に知らせる。 */
export function refreshTagKinds(): Promise<TagKind[]> {
  return startFetch();
}

/** subscribeTagKinds は共有の保持が取り直されるたびに呼ばれる。戻り値で購読をやめる。 */
export function subscribeTagKinds(listener: TagKindsListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * afterTagKindsChanged は種類を変える操作の後に呼ぶ。購読者がいれば取り直し、いなければ
 * 保持を捨てて次の `getTagKinds` に取らせる（進行中の取得は届いても `held` に入れない）。
 * 完了は待たず、失敗しても揉み消す。
 */
function afterTagKindsChanged(): void {
  held = undefined;
  latestFetch = undefined;
  generation += 1;
  if (listeners.size > 0) refreshTagKinds().catch(() => undefined);
}

/**
 * afterTagKindRenamedOrDeleted は種類の名前の変更と削除の後に呼ぶ。キャッシュしたタグは
 * どれも種類の名前を持つので、共有のタグの一覧を取り直し、動画一覧の控えを捨てる
 * （specs/051-tag-kinds/research.md R-4）。
 */
function afterTagKindRenamedOrDeleted(): void {
  afterTagKindsChanged();
  clearListSnapshot();
  refreshTags().catch(() => undefined);
}

/**
 * refreshOnStaleTagKindError は `tag_kind_not_found` を受けたときに種類の一覧を取り直す
 * （別のタブで消された種類に、その場で気付けるようにする）。誤りは常にそのまま投げ直す。
 */
function refreshOnStaleTagKindError(error: unknown): never {
  if (error instanceof RequestFailed && error.reason === "tag_kind_not_found") {
    afterTagKindsChanged();
  }
  throw error;
}

/** createTagKind は種類を 1 件作る（`POST /api/tag-kinds`）。 */
export async function createTagKind(
  name: string,
  signal?: AbortSignal,
): Promise<TagKind> {
  const created = await request<TagKind>("/api/tag-kinds", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  });
  afterTagKindsChanged();
  return created;
}

/**
 * renameTagKind は種類の名前を書き換える（`PATCH /api/tag-kinds/{id}`）。今と同じ名前なら
 * 何も変えずに今の状態が返る。
 */
export async function renameTagKind(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<TagKind> {
  const updated = await request<TagKind>(`/api/tag-kinds/${String(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  }).catch(refreshOnStaleTagKindError);
  afterTagKindRenamedOrDeleted();
  return updated;
}

/**
 * deleteTagKind は種類を消す（`DELETE /api/tag-kinds/{id}`）。その種類のタグは種類なしで残る。
 */
export async function deleteTagKind(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/tag-kinds/${String(id)}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    refreshOnStaleTagKindError(await toRequestFailed(response));
  }
  afterTagKindRenamedOrDeleted();
}

/**
 * __resetTagKindsForTest はテストだけで使う。Vitest はテストファイル単位でモジュールを
 * 使い回すので、テストどうしで保持が残らないようにここでだけ空へ戻す。
 */
export function __resetTagKindsForTest(): void {
  held = undefined;
  generation = 0;
  latestFetch = undefined;
  listeners.clear();
}
