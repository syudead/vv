import type { components } from "./gen/openapi";

// 型は api/openapi.yaml からの生成物を使う。契約を変えると、ここが
// コンパイルエラーになって気付ける（R-010）。
export type Video = components["schemas"]["Video"];
export type VideoPage = components["schemas"]["VideoPage"];
export type VideoSort = components["schemas"]["VideoSort"];
export type Scan = components["schemas"]["Scan"];
export type Progress = components["schemas"]["Progress"];
export type ApiError = components["schemas"]["Error"];

// 1ページの件数。既定は契約（api/openapi.yaml）と同じ 60 で、最初の画面は
// これだけを待つ（R-114 / SC-003）。
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
 * 詳細・進捗送信」の3種類しかなく、キャッシュ無効化の関係も単純だからである
 * （R-113）。画面をまたぐキャッシュ整合や楽観更新が要る段階で再検討する。
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
  return request<Scan>("/api/scans", { method: "POST", signal });
}

/**
 * saveProgress は再生位置を送る。視聴済みの判定はサーバー側が行うので、
 * ここでは位置だけを送る（R-111）。
 */
export function saveProgress(
  id: number,
  positionMs: number,
  signal?: AbortSignal,
): Promise<Progress> {
  return request<Progress>(`/api/videos/${String(id)}/progress`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ positionMs: Math.max(0, Math.round(positionMs)) }),
    signal,
  });
}

/**
 * beaconProgress は離脱時に再生位置を送る。
 *
 * 画面を閉じる途中では fetch が最後まで走る保証が無い。sendBeacon は
 * ブラウザが送信を引き受けるので、閉じる操作で記録を取りこぼさない。
 * Content-Type を選べず text/plain になるため、サーバー側は双方を受理する
 * （contracts/http-routes.md）。
 */
export function beaconProgress(id: number, positionMs: number): boolean {
  const body = JSON.stringify({ positionMs: Math.max(0, Math.round(positionMs)) });

  if (typeof navigator.sendBeacon === "function") {
    return navigator.sendBeacon(`/api/videos/${String(id)}/progress`, body);
  }

  // sendBeacon が無い環境では、届かないよりは試みる方がよい。
  void saveProgress(id, positionMs).catch(() => undefined);
  return false;
}

/** streamUrl は動画本体の取得先を返す。 */
export function streamUrl(id: number): string {
  return `/api/videos/${id}/stream`;
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
