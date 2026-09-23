import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { nextProgressSequence, recordSavedProgress } from "./progressEvents";

// 型は api/openapi.yaml からの生成物を使う。契約を変えると、ここが
// コンパイルエラーになって気付ける。
export type Video = components["schemas"]["Video"];
export type VideoPage = components["schemas"]["VideoPage"];
export type VideoSort = components["schemas"]["VideoSort"];
export type Scan = components["schemas"]["Scan"];
export type Progress = components["schemas"]["Progress"];
export type MediaFolder = components["schemas"]["MediaFolder"];
export type DirectoryListing = components["schemas"]["DirectoryListing"];
export type ApiError = components["schemas"]["Error"];
export type FolderSummary = components["schemas"]["FolderSummary"];
export type FolderListing = components["schemas"]["FolderListing"];
export type RootFolderListing = components["schemas"]["RootFolderListing"];

// 1ページの件数。既定は契約（api/openapi.yaml）と同じ 60 で、最初の画面は
// これだけを待つ。
export const PAGE_SIZE = 60;

/** 応答がエラーだったことを表す。message は利用者にそのまま見せてよい。 */
export class RequestFailed extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "RequestFailed";
    this.status = status;
    this.code = code;
  }
}

/**
 * request は JSON を取りに行く薄いラッパである。
 *
 * データ取得ライブラリは入れない。この時点の要求は「一覧（無限スクロール）・
 * 詳細・進捗送信」の3種類しかなく、キャッシュ無効化の関係も単純だからである。
 * 画面をまたぐキャッシュ整合や楽観更新が要る段階で再検討する。
 */
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);

  if (!response.ok) {
    throw await toRequestFailed(response);
  }
  return (await response.json()) as T;
}

/** toRequestFailed は誤りの応答を、画面に出せる形へ変える。 */
async function toRequestFailed(response: Response): Promise<RequestFailed> {
  try {
    const body = (await response.json()) as ApiError;
    if (typeof body?.message === "string" && body.message !== "") {
      return new RequestFailed(response.status, body.code ?? "internal", body.message);
    }
  } catch {
    // JSON で返ってこない場合もある（経路の取り違え、中継の失敗）。
  }
  return new RequestFailed(
    response.status,
    "internal",
    `要求に失敗しました (${response.status})`,
  );
}

/** MAX_QUERY_LENGTH は検索語に許す長さである（api/openapi.yaml の maxLength）。 */
export const MAX_QUERY_LENGTH = 100;

/** ListVideosParams は一覧の問い合わせ条件である。 */
export interface ListVideosParams {
  query?: string;
  sort?: VideoSort;
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

/** listVideos は一覧を1ページ取得する。 */
export function listVideos(params: ListVideosParams = {}): Promise<VideoPage> {
  const query = new URLSearchParams();
  if (params.query !== undefined && params.query !== "") {
    query.set("query", params.query.slice(0, MAX_QUERY_LENGTH));
  }
  if (params.sort !== undefined) {
    query.set("sort", params.sort);
  }
  if (params.cursor !== undefined && params.cursor !== "") {
    query.set("cursor", params.cursor);
  }
  query.set("limit", String(params.limit ?? PAGE_SIZE));

  return request<VideoPage>(`/api/videos?${query.toString()}`, { signal: params.signal });
}

/** FolderRef はフォルダを指す。登録フォルダの id と `/` 区切りの相対パスの組である。 */
export interface FolderRef {
  rootId: number;
  /** 登録フォルダ自身は空文字。 */
  path: string;
}

/** listRootFolders はフォルダ画面の最上位（登録済みメディアフォルダ）を取得する。 */
export function listRootFolders(signal?: AbortSignal): Promise<RootFolderListing> {
  return request<RootFolderListing>("/api/folders", { signal });
}

/** folderQuery は相対パスを問い合わせに載せる。登録フォルダ自身では省く。 */
function folderQuery(folder: FolderRef): URLSearchParams {
  const query = new URLSearchParams();
  if (folder.path !== "") query.set("path", folder.path);
  return query;
}

/** getFolder はフォルダ1件と直下の子フォルダを取得する。 */
export function getFolder(
  folder: FolderRef,
  signal?: AbortSignal,
): Promise<FolderListing> {
  const query = folderQuery(folder);
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return request<FolderListing>(`/api/folders/${String(folder.rootId)}${suffix}`, {
    signal,
  });
}

/** ListFolderVideosParams はフォルダ直下の動画の問い合わせ条件である。 */
export interface ListFolderVideosParams {
  folder: FolderRef;
  sort?: VideoSort;
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

/** listFolderVideos はフォルダ直下の動画を1ページ取得する。 */
export function listFolderVideos(params: ListFolderVideosParams): Promise<VideoPage> {
  const query = folderQuery(params.folder);
  if (params.sort !== undefined) query.set("sort", params.sort);
  if (params.cursor !== undefined && params.cursor !== "")
    query.set("cursor", params.cursor);
  query.set("limit", String(params.limit ?? PAGE_SIZE));
  return request<VideoPage>(
    `/api/folders/${String(params.folder.rootId)}/videos?${query.toString()}`,
    { signal: params.signal },
  );
}

/** getVideo は動画1件の詳細を取得する。 */
export function getVideo(id: number, signal?: AbortSignal): Promise<Video> {
  return request<Video>(`/api/videos/${id}`, { signal });
}

/** getCurrentScan は直近の取り込みの状態を取得する。一度も取り込んでいなければ null。 */
export async function getCurrentScan(signal?: AbortSignal): Promise<Scan | null> {
  try {
    return await request<Scan>("/api/scans/current", { signal });
  } catch (error) {
    if (error instanceof RequestFailed && error.status === 404) {
      return null;
    }
    throw error;
  }
}

/** startScan は取り込みを促す。実行中なら、実行中のものがそのまま返る。 */
export function startScan(signal?: AbortSignal): Promise<Scan> {
  return request<Scan>("/api/scans", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
    signal,
  });
}

/** listMediaFolders は登録rootをid順で取得する。 */
export function listMediaFolders(signal?: AbortSignal): Promise<MediaFolder[]> {
  return request<MediaFolder[]>("/api/media-folders", { signal });
}

/**
 * createMediaFolder は選択済みpathを1件追加する。
 *
 * 登録フォルダを変える3つの操作は、成功したら一覧の控えを捨てる。控えは
 * 登録の変更を知らないので、戻ったときに消えたフォルダや古い一覧を出してしまう。
 */
export async function createMediaFolder(
  path: string,
  signal?: AbortSignal,
): Promise<MediaFolder> {
  const created = await request<MediaFolder>("/api/media-folders", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path }),
    signal,
  });
  clearListSnapshot();
  return created;
}

/** updateMediaFolder は1件だけをversion付きで置き換える。 */
export async function updateMediaFolder(
  id: number,
  path: string,
  version: number,
  signal?: AbortSignal,
): Promise<MediaFolder> {
  const updated = await request<MediaFolder>(`/api/media-folders/${String(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, version }),
    signal,
  });
  clearListSnapshot();
  return updated;
}

/** deleteMediaFolder は1件だけをversion付きで削除する。 */
export async function deleteMediaFolder(
  id: number,
  version: number,
  signal?: AbortSignal,
): Promise<void> {
  const response = await fetch(
    `/api/media-folders/${String(id)}?version=${String(version)}`,
    { method: "DELETE", signal },
  );
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
  clearListSnapshot();
}

/** listDirectories はpickerのnavigation起点または指定path直下を取得する。 */
export function listDirectories(
  path?: string,
  signal?: AbortSignal,
): Promise<DirectoryListing> {
  const query = new URLSearchParams();
  if (path !== undefined) {
    query.set("path", path);
  }
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return request<DirectoryListing>(`/api/directories${suffix}`, { signal });
}

/**
 * progressQueue は動画ごとに、送った保存の完了を待つ連なりである。
 *
 * サーバーは届いた保存をそのまま上書きするので、同じ動画の保存を並行して送ると、
 * 先に送った古い位置が後から届いて残ることがある。1件ずつ順に送る。
 */
const progressQueue = new Map<number, Promise<unknown>>();

function enqueueProgress<T>(id: number, send: () => Promise<T>): Promise<T> {
  // 送信中の保存が無ければ、その場で送る（画面を離れる瞬間の送信を遅らせない）。
  const previous = progressQueue.get(id);
  const next = previous === undefined ? send() : previous.then(send);
  const tail = next.catch(() => undefined);
  progressQueue.set(id, tail);
  void tail.then(() => {
    if (progressQueue.get(id) === tail) progressQueue.delete(id);
  });
  return next;
}

/**
 * saveProgress は再生位置を送る。視聴済みの判定はサーバー側が行うので、
 * ここでは位置だけを送る。同じ動画の保存は、前の保存が終わってから送る。
 */
export function saveProgress(
  id: number,
  positionMs: number,
  signal?: AbortSignal,
): Promise<Progress> {
  const sequence = nextProgressSequence();
  return enqueueProgress(id, async () => {
    const saved = await request<Progress>(`/api/videos/${String(id)}/progress`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ positionMs: Math.max(0, Math.round(positionMs)) }),
      signal,
    });
    recordSavedProgress(id, saved, sequence);
    return saved;
  });
}

/**
 * beaconProgress は離脱時に再生位置を送る。
 *
 * keepalive付きfetchで、既存のPUT契約を保ったまま画面離脱後も送信を継続する。
 * アプリの中で画面を移るときは、送信中の保存が終わってから送る。ページ自体が
 * 隠れる（タブを閉じるなど）ときは待てないので、すぐに送る。
 */
export function beaconProgress(id: number, positionMs: number): void {
  const body = JSON.stringify({ positionMs: Math.max(0, Math.round(positionMs)) });
  const sequence = nextProgressSequence();
  const send = () =>
    fetch(`/api/videos/${String(id)}/progress`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body,
      keepalive: true,
    }).then(async (response) => {
      if (response.ok)
        recordSavedProgress(id, (await response.json()) as Progress, sequence);
    });
  const sending =
    document.visibilityState === "hidden" ? send() : enqueueProgress(id, send);
  void sending.catch(() => undefined);
}

/** streamUrl は動画本体の取得先を返す。 */
export function streamUrl(id: number): string {
  return `/api/videos/${id}/stream`;
}

/** transcodeUrl は指定した元動画時刻からライブ変換する取得先を返す。 */
export function transcodeUrl(id: number, startMs = 0): string {
  const query = new URLSearchParams();
  if (startMs > 0) query.set("startMs", String(Math.round(startMs)));
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return `/api/videos/${String(id)}/transcode.mp4${suffix}`;
}

/** fetchSeekThumbnail はシークプレビュー用のJPEGを取得する。 */
export async function fetchSeekThumbnail(
  url: string,
  signal: AbortSignal,
): Promise<Blob> {
  const response = await fetch(url, { signal });
  if (!response.ok) {
    throw new RequestFailed(
      response.status,
      "seek_thumbnail_failed",
      `seek thumbnail request failed: ${String(response.status)}`,
    );
  }
  return response.blob();
}

/** isAborted は「利用者が先に進んだので打ち切った」だけかどうかを返す。 */
export function isAborted(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

/** errorMessage は画面に出す文言を取り出す。 */
export function errorMessage(error: unknown): string {
  if (error instanceof RequestFailed) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
