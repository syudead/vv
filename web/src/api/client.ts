import { reloadPage } from "../auth/pageNavigation";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { nextProgressSequence, recordSavedProgress } from "./progressEvents";

// 型は api/openapi.yaml からの生成物を使う。契約を変えると、ここが
// コンパイルエラーになって気付ける。
export type Video = components["schemas"]["Video"];
export type VideoPage = components["schemas"]["VideoPage"];
export type VideoSort = components["schemas"]["VideoSort"];

/** videoSorts は API が受け付ける並び順のすべてである（list-api.md §3）。 */
export const videoSorts: readonly VideoSort[] = [
  "addedAsc",
  "addedDesc",
  "modifiedAsc",
  "modifiedDesc",
  "titleAsc",
  "titleDesc",
  "durationAsc",
  "durationDesc",
  "sizeAsc",
  "sizeDesc",
  "playedAsc",
  "playedDesc",
  "random",
];

/** isVideoSort は API が受け付ける並び順かどうかを返す。 */
export function isVideoSort(value: unknown): value is VideoSort {
  return typeof value === "string" && (videoSorts as readonly string[]).includes(value);
}
export type WatchFilter = components["schemas"]["WatchFilter"];
export type TagRef = components["schemas"]["TagRef"];
export type VideoTag = components["schemas"]["VideoTag"];
export type VideoIdsResponse = components["schemas"]["VideoIdsResponse"];
export type FolderScope = components["schemas"]["FolderScope"];
export type VideoFolder = components["schemas"]["VideoFolder"];
export type Scan = components["schemas"]["Scan"];
export type Progress = components["schemas"]["Progress"];
export type MediaFolder = components["schemas"]["MediaFolder"];
export type DirectoryListing = components["schemas"]["DirectoryListing"];
export type ApiError = components["schemas"]["Error"];
export type FolderSummary = components["schemas"]["FolderSummary"];
export type FolderListing = components["schemas"]["FolderListing"];
export type RootFolderListing = components["schemas"]["RootFolderListing"];
export type RelatedVideos = components["schemas"]["RelatedVideos"];
export type VideoLocation = components["schemas"]["VideoLocation"];
export type LibraryGroup = components["schemas"]["LibraryGroup"];

/**
 * LibraryItem は一覧の項目1件である（api/openapi.yaml の LibraryItem）。生成した型は
 * `video` と `group` をどちらも省略可能にしているので、`kind` で読み分けられる形にする
 * （specs/017-folder-groups/plan.md の Structural Decisions 13）。
 */
export type LibraryItem =
  { kind: "video"; video: Video } | { kind: "group"; group: LibraryGroup };
export type Processing = components["schemas"]["Processing"];
export type VideoChanged = components["schemas"]["VideoChanged"];

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

// --- 見る人が変わったときの読み直し（specs/016-single-account-auth/plan.md
// Structural Decisions 14、ui-design.md「Gate」） ---

/** renderedAudience はゲートが確かめた、今描いている相手である。確かめる前は null。 */
let renderedAudience: "owner" | "guest" | null = null;
/** reloading は読み直しを1度始めたかどうか。2度目からは何もしない。 */
let reloading = false;

/**
 * setRenderedAudience は、ゲートが確かめた相手を覚える。所有者として描いている間に
 * 見る人が変わったと分かったら、apiFetch がページを読み直す。
 */
export function setRenderedAudience(audience: "owner" | "guest" | null): void {
  renderedAudience = audience;
  reloading = false;
}

/**
 * reloadForViewerChange は、見る人が変わったときに今の URL をページごと1度だけ
 * 読み直す。何度呼んでも読み直しは1回である。トーストは出さない。読み直した画面は
 * ゲートで状態を取り直す。
 */
export function reloadForViewerChange(): void {
  if (reloading) return;
  reloading = true;
  reloadPage();
}

/**
 * reloadIfNoLongerOwner は、所有者として描いている間に確かめた見る人の状態が所有者で
 * なくなっていたら、ページを1度だけ読み直して true を返す。応答の本文や状態を
 * 読めない経路（`/api/events` の EventSource、`video` 要素）が、状態の確認
 * （GET /api/auth/session）の答えを渡すために使う。
 */
export function reloadIfNoLongerOwner(state: string): boolean {
  if (renderedAudience !== "owner" || state === "owner") return false;
  reloadForViewerChange();
  return true;
}

/**
 * viewerChanged は、所有者として描いている間に、この応答が「もう所有者ではない」
 * ことを示すか（401 か X-VV-Audience: guest）を返す。
 */
function viewerChanged(response: Response): boolean {
  if (renderedAudience !== "owner") return false;
  return response.status === 401 || response.headers.get("X-VV-Audience") === "guest";
}

/**
 * apiFetch は `/api/*` への fetch である。所有者として描いている間に 401 か
 * `X-VV-Audience: guest` を受けたら、ページを1度だけ読み直し、応答は返さない
 * （決して解決しない）。ゲストとして処理した応答を、所有者の画面に一瞬でも
 * 描かないためである。認証の経路（auth.ts）はこれを通さない。
 */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(path, init);
  if (viewerChanged(response)) {
    reloadForViewerChange();
    return new Promise<Response>(() => undefined);
  }
  return response;
}

/**
 * request は JSON を取りに行く薄いラッパである。
 *
 * データ取得ライブラリは入れない。この時点の要求は「一覧（無限スクロール）・
 * 詳細・進捗送信」の3種類しかなく、キャッシュ無効化の関係も単純だからである。
 * 画面をまたぐキャッシュ整合や楽観更新が要る段階で再検討する。
 */
export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await apiFetch(path, init);

  if (!response.ok) {
    throw await toRequestFailed(response);
  }
  return (await response.json()) as T;
}

/** toRequestFailed は誤りの応答を、画面に出せる形へ変える。 */
export async function toRequestFailed(response: Response): Promise<RequestFailed> {
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

/**
 * ListFilterParams は2つの一覧（ライブラリとフォルダ）が共通に受ける条件である
 * （specs/013-library-search/contracts/list-api.md §2・§3）。
 */
export interface ListFilterParams {
  /** 検索語。空なら送らない。100 文字を越える分は切り詰める。 */
  query?: string;
  /** 視聴状態の絞り込み。省略時はサーバーの既定（all）。 */
  watch?: WatchFilter;
  /** true ならブラウザで再生できる動画だけにする。false は既定なので送らない。 */
  playable?: boolean;
  sort?: VideoSort;
  /** sort=random の並びを決める値（0 以上 2147483647 以下）。 */
  seed?: number;
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

/** MAX_TAG_FILTER_COUNT はタグでの絞り込みに使える id の最大個数である。 */
export const MAX_TAG_FILTER_COUNT = 16;

/**
 * ListVideosParams は一覧の問い合わせ条件である。`tag` は listFolderVideos には
 * 無い（フォルダ画面のタグ絞り込みは対象外。
 * specs/014-video-tags/contracts/tags-api.md §5）。
 */
export interface ListVideosParams extends ListFilterParams {
  /** 絞り込むタグの id（すべて持つ動画だけにする AND）。最大16個。書く順は問わない。 */
  tag?: number[];
}

/** setListFilters は共通の条件を問い合わせに載せる。 */
function setListFilters(query: URLSearchParams, params: ListFilterParams): void {
  if (params.query !== undefined && params.query !== "") {
    // サーバーと同じく符号位置で数えて切る（サロゲートペアを割らない）。
    query.set("query", Array.from(params.query).slice(0, MAX_QUERY_LENGTH).join(""));
  }
  if (params.watch !== undefined) query.set("watch", params.watch);
  if (params.playable === true) query.set("playable", "true");
  if (params.sort !== undefined) query.set("sort", params.sort);
  if (params.seed !== undefined) query.set("seed", String(params.seed));
  if (params.cursor !== undefined && params.cursor !== "") {
    query.set("cursor", params.cursor);
  }
  query.set("limit", String(params.limit ?? PAGE_SIZE));
}

/** setTagFilter はタグの絞り込みを問い合わせに載せる（listVideos・listVideoIds）。 */
function setTagFilter(query: URLSearchParams, tag?: number[]): void {
  if (tag === undefined) return;
  for (const id of tag.slice(0, MAX_TAG_FILTER_COUNT)) {
    query.append("tag", String(id));
  }
}

/** listVideos は一覧を1ページ取得する。 */
export function listVideos(params: ListVideosParams = {}): Promise<VideoPage> {
  const query = new URLSearchParams();
  setListFilters(query, params);
  setTagFilter(query, params.tag);
  return request<VideoPage>(`/api/videos?${query.toString()}`, { signal: params.signal });
}

/** ListVideoIdsParams は「すべて選択」用の全件 id の問い合わせ条件である。 */
export interface ListVideoIdsParams {
  query?: string;
  watch?: WatchFilter;
  playable?: boolean;
  tag?: number[];
  signal?: AbortSignal;
}

/**
 * listVideoIds は listVideos と同じ条件に合う全件の id を、ページングせずに
 * 取得する（「すべて選択」用。specs/014-video-tags/contracts/tags-api.md §5）。
 */
export function listVideoIds(params: ListVideoIdsParams = {}): Promise<VideoIdsResponse> {
  const query = new URLSearchParams();
  if (params.query !== undefined && params.query !== "") {
    query.set("query", Array.from(params.query).slice(0, MAX_QUERY_LENGTH).join(""));
  }
  if (params.watch !== undefined) query.set("watch", params.watch);
  if (params.playable === true) query.set("playable", "true");
  setTagFilter(query, params.tag);
  return request<VideoIdsResponse>(`/api/videos/ids?${query.toString()}`, {
    signal: params.signal,
  });
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

/** ListFolderVideosParams はフォルダの動画の問い合わせ条件である。 */
export interface ListFolderVideosParams extends ListFilterParams {
  folder: FolderRef;
  /** direct（既定）は直下だけ、subtree は配下すべて。省略時は送らない。 */
  scope?: FolderScope;
}

/** listFolderVideos はフォルダの動画を1ページ取得する。 */
export function listFolderVideos(params: ListFolderVideosParams): Promise<VideoPage> {
  const query = folderQuery(params.folder);
  if (params.scope !== undefined) query.set("scope", params.scope);
  setListFilters(query, params);
  return request<VideoPage>(
    `/api/folders/${String(params.folder.rootId)}/videos?${query.toString()}`,
    { signal: params.signal },
  );
}

/**
 * getFolderGroup はフォルダのグループ1件を、絞り込みに関係なく全メンバーから取得する
 * （一覧に残っているグループの項目の取り直し。specs/017-folder-groups/contracts/library-api.md §3）。
 * そのフォルダが今グループでないか無いときは 404 の RequestFailed で失敗する。
 */
export function getFolderGroup(
  folder: FolderRef,
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  const query = folderQuery(folder);
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return request<LibraryGroup>(`/api/folders/${String(folder.rootId)}/group${suffix}`, {
    signal,
  });
}

/** getVideo は動画1件の詳細を取得する。 */
export function getVideo(id: number, signal?: AbortSignal): Promise<Video> {
  return request<Video>(`/api/videos/${id}`, { signal });
}

/** getRelatedVideos は動画詳細画面の関連動画（最大 20 件と次の動画の id）を取得する。 */
export function getRelatedVideos(
  id: number,
  signal?: AbortSignal,
): Promise<RelatedVideos> {
  return request<RelatedVideos>(`/api/videos/${String(id)}/related`, { signal });
}

/**
 * reprobeVideo は読み取りに失敗した動画を、もう一度読み取りの処理へ入れる。
 *
 * 202 の本文は所在とシーク用プレビューの状態を持たない（動画 1 件の取得にだけ入る）。
 * 画面はこの本文で控えを置き換えず、取り直して使う。
 */
export function reprobeVideo(id: number, signal?: AbortSignal): Promise<Video> {
  return request<Video>(`/api/videos/${String(id)}/probe`, { method: "POST", signal });
}

/** openVideoFile はサーバーの PC で、動画の代表の所在を既定のアプリで開く。 */
export async function openVideoFile(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/videos/${String(id)}/open`, {
    method: "POST",
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
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

/** getProcessing は取り込みの段階ごとに残っている仕事の数を取得する。 */
export function getProcessing(signal?: AbortSignal): Promise<Processing> {
  return request<Processing>("/api/processing", { signal });
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
  const response = await apiFetch(
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
 * sendProgressNow は順番を待たずにすぐ送り、それでも以後の保存は、それまでの
 * 保存とこの送信の両方が終わってから送るようにする。
 */
function sendProgressNow<T>(id: number, send: () => Promise<T>): Promise<T> {
  const previous = progressQueue.get(id) ?? Promise.resolve();
  const next = send();
  const tail = Promise.all([previous, next.catch(() => undefined)]);
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
 * 隠れる（タブを閉じるなど）ときは待てないので、すぐに送る。その場合も、以後の
 * 保存はこの送信の完了を待つ。すでに送信中だった保存との順序は、クライアントだけ
 * では保証できない（サーバーは届いた順に上書きする）。
 */
export function beaconProgress(id: number, positionMs: number): void {
  const body = JSON.stringify({ positionMs: Math.max(0, Math.round(positionMs)) });
  const sequence = nextProgressSequence();
  const send = () =>
    apiFetch(`/api/videos/${String(id)}/progress`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body,
      keepalive: true,
    }).then(async (response) => {
      if (response.ok)
        recordSavedProgress(id, (await response.json()) as Progress, sequence);
    });
  const sending =
    document.visibilityState === "hidden"
      ? sendProgressNow(id, send)
      : enqueueProgress(id, send);
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
  const response = await apiFetch(url, { signal });
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
