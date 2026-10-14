import { request, type Video } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";

// 型は api/openapi.yaml からの生成物を使う（specs/047-video-work-info/contracts/screen-api.md）。
export type WorkInfo = components["schemas"]["WorkInfo"];
export type WorkInfoInput = components["schemas"]["WorkInfoInput"];
export type WorkDateKind = components["schemas"]["WorkDateKind"];
export type WorkPersonRole = components["schemas"]["WorkPersonRole"];
export type WorkOrganizationRole = components["schemas"]["WorkOrganizationRole"];
export type VideoDetailsInput = components["schemas"]["VideoDetailsInput"];

/** workDateKinds は日付の種類の並び（WorkInfo の `dates` の順）である。 */
export const workDateKinds: readonly WorkDateKind[] = [
  "release",
  "recorded",
  "published",
];

/** workPersonRoles は人物の役割の並び（WorkInfo の `people` の順）である。 */
export const workPersonRoles: readonly WorkPersonRole[] = [
  "performer",
  "director",
  "photographer",
  "streamer",
];

/** workOrganizationRoles は団体の役割の並び（WorkInfo の `organizations` の順）である。 */
export const workOrganizationRoles: readonly WorkOrganizationRole[] = ["maker", "label"];

/**
 * setVideoDetails は全画面の編集画面の保存で、表示名と作品情報の全体を 1 回で書く
 * （`PUT /api/videos/{id}/details`、contracts/screen-api.md、research.md R-14）。サーバーは
 * 両方を書くか、どちらも書かない。応答は `getVideo` と同じ形の動画である。
 *
 * 成功したら一覧の控えを捨てる。控えは離れた一覧の題名を持ち、一覧が外れている間の
 * `video` 通知では直らないので、戻ったときは読み直す（setVideoDisplayName と同じ）。
 */
export async function setVideoDetails(
  id: number,
  body: VideoDetailsInput,
  signal?: AbortSignal,
): Promise<Video> {
  const video = await request<Video>(`/api/videos/${String(id)}/details`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  clearListSnapshot();
  return video;
}
