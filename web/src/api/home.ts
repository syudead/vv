import {
  apiFetch,
  type LibraryItem,
  request,
  toLibraryItems,
  toRequestFailed,
} from "./client";
import type { components } from "./gen/openapi";

// ホームの画面 API（specs/057-home/contracts/screen-api.md）。型は api/openapi.yaml からの
// 生成物を使う。棚の一覧（GET /api/home）を先に読み、棚の項目は画面に入った棚の分だけ
// GET /api/home/shelves で読む（research.md R-12）。どちらもゲストも読める。

export type Home = components["schemas"]["Home"];
export type HomeShelf = components["schemas"]["HomeShelf"];
export type HomeShelfKey = components["schemas"]["HomeShelfKey"];
/** HomeShelfItems は棚 1 つの最初の項目である。項目は `kind` で読み分けられる形にする。 */
export type HomeShelfItems = Omit<components["schemas"]["HomeShelfItems"], "items"> & {
  items: LibraryItem[];
};

/** MAX_HOME_SHELF_KEYS は readHomeShelves 1 回で名指せる棚の数の上限である。 */
export const MAX_HOME_SHELF_KEYS = 8;

/** readHome は見る人のホームの棚の一覧を、上から順に読む（GET /api/home）。 */
export function readHome(signal?: AbortSignal): Promise<Home> {
  return request<Home>("/api/home", { signal });
}

/**
 * readHomeShelves は `keys` の棚それぞれの最初の項目と項目の数を `keys` の順に読む
 * （GET /api/home/shelves）。もう無い棚の鍵は応答から落ちる。`keys` は 1〜8 件。
 */
export async function readHomeShelves(
  keys: readonly HomeShelfKey[],
  signal?: AbortSignal,
): Promise<HomeShelfItems[]> {
  const query = keys.map(encodeURIComponent).join(",");
  const list = await request<components["schemas"]["HomeShelfItemsList"]>(
    `/api/home/shelves?keys=${query}`,
    { signal },
  );
  return list.shelves.map((shelf) => ({ ...shelf, items: toLibraryItems(shelf.items) }));
}

export type HomeOrganizer = components["schemas"]["HomeOrganizer"];
export type HomeOrganizeRun = components["schemas"]["HomeOrganizeRun"];

/**
 * setHomeShelves はフォルダの棚を `folderIds` の順にする（PUT /api/home/shelves）。名指さない
 * フォルダは棚から外れる（research.md R-7）。所有者だけ。
 */
export async function setHomeShelves(
  folderIds: readonly number[],
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch("/api/home/shelves", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ folderIds }),
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
}

/**
 * organizeHome はホームの組み直し（rebuild）を待ちに入れ、待ちに入れたあとの整理役の状態を
 * 返す（POST /api/home/organize）。所有者だけ（research.md R-9）。
 */
export function organizeHome(signal?: AbortSignal): Promise<HomeOrganizer> {
  return request<HomeOrganizer>("/api/home/organize", { method: "POST", signal });
}
