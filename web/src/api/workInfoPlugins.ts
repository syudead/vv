import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { afterTagsAppliedByName } from "./tags";

// 作品情報プラグインの画面 API（specs/049-work-info-plugins/contracts/screen-api.md）。型は
// api/openapi.yaml からの生成物を使う。どの経路も所有者だけである。プラグインの変化はイベントで
// 流れないので、画面は操作のあとに listWorkInfoPlugins で読み直す。

export type WorkInfoPlugin = components["schemas"]["WorkInfoPlugin"];
export type WorkInfoPluginSetting = components["schemas"]["WorkInfoPluginSetting"];
export type WorkInfoPluginState = components["schemas"]["WorkInfoPluginState"];
export type WorkInfoPluginProblem = components["schemas"]["WorkInfoPluginProblem"];
export type WorkInfoPluginList = components["schemas"]["WorkInfoPluginList"];
export type WorkInfoCandidate = components["schemas"]["WorkInfoCandidate"];
export type WorkInfoFetch = components["schemas"]["WorkInfoFetch"];
export type WorkInfoField = components["schemas"]["WorkInfoField"];
export type WorkInfoFetchApplied = components["schemas"]["WorkInfoFetchApplied"];
export type SkippedTag = components["schemas"]["SkippedTag"];

const jsonHeaders = { "Content-Type": "application/json" };

function pluginPath(id: number): string {
  return `/api/work-info-plugins/${String(id)}`;
}

/** listWorkInfoPlugins はプラグインを名前、次にファイル名の順で取得する（GET /api/work-info-plugins）。 */
export async function listWorkInfoPlugins(
  signal?: AbortSignal,
): Promise<WorkInfoPlugin[]> {
  const list = await request<WorkInfoPluginList>("/api/work-info-plugins", { signal });
  return list.plugins;
}

/**
 * uploadWorkInfoPlugin は `.wasm` のファイルを `multipart/form-data` の `module` で送り、プラグインを
 * 追加するか、マニフェストの名前が同じプラグインと差し替える（POST /api/work-info-plugins）。使えない
 * ファイルも `state: "invalid"` で返る。16 MiB を超えれば 413 `plugin_too_large` で失敗する。Content-Type は
 * ブラウザが境界とともに付けるので、ここでは付けない。
 */
export function uploadWorkInfoPlugin(
  module: File,
  signal?: AbortSignal,
): Promise<WorkInfoPlugin> {
  const form = new FormData();
  form.append("module", module, module.name);
  return request<WorkInfoPlugin>("/api/work-info-plugins", {
    method: "POST",
    body: form,
    signal,
  });
}

/**
 * approveWorkInfoPlugin は一覧で見た `digest` のモジュールを承認する（PUT …/{id}/approval）。差し替え
 * られていれば 409 `plugin_changed`、使えないプラグインなら 409 `plugin_unusable` で失敗する。
 */
export function approveWorkInfoPlugin(
  id: number,
  digest: string,
  signal?: AbortSignal,
): Promise<WorkInfoPlugin> {
  return request<WorkInfoPlugin>(`${pluginPath(id)}/approval`, {
    method: "PUT",
    headers: jsonHeaders,
    body: JSON.stringify({ digest }),
    signal,
  });
}

/** revokeWorkInfoPluginApproval はプラグインの承認を取り消す（DELETE …/{id}/approval）。 */
export function revokeWorkInfoPluginApproval(
  id: number,
  signal?: AbortSignal,
): Promise<WorkInfoPlugin> {
  return request<WorkInfoPlugin>(`${pluginPath(id)}/approval`, {
    method: "DELETE",
    signal,
  });
}

/**
 * setWorkInfoPluginSettings は設定の値を書く（PUT …/{id}/settings）。`null` はその値を消し、省いた
 * キーはそのまま残る。承認は変わらない。応答の秘密の設定は `set` だけを持つ。
 */
export function setWorkInfoPluginSettings(
  id: number,
  values: Readonly<Record<string, string | null>>,
  signal?: AbortSignal,
): Promise<WorkInfoPlugin> {
  return request<WorkInfoPlugin>(`${pluginPath(id)}/settings`, {
    method: "PUT",
    headers: jsonHeaders,
    body: JSON.stringify({ values }),
    signal,
  });
}

/** deleteWorkInfoPlugin はプラグインとその設定の値を消す（DELETE …/{id}）。無ければ 404 の RequestFailed。 */
export async function deleteWorkInfoPlugin(
  id: number,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(pluginPath(id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/**
 * searchWorkInfo は承認済みのプラグインの search で作品を探し、候補を返す（POST …/{id}/search）。`query` は
 * 前後の空白を除いて 1〜200 文字である。失敗は Run failures の RequestFailed（`pluginMessage`・`host`・
 * `settingKeys` を持ちうる）で、`search` を持たないプラグインは 409 `operation_unsupported` である。
 */
export async function searchWorkInfo(
  id: number,
  query: string,
  signal?: AbortSignal,
): Promise<WorkInfoCandidate[]> {
  const result = await request<components["schemas"]["WorkInfoSearchResult"]>(
    `${pluginPath(id)}/search`,
    { method: "POST", headers: jsonHeaders, body: JSON.stringify({ query }), signal },
  );
  return result.candidates;
}

/**
 * fetchWorkInfo は承認済みのプラグイン `pluginId` の fetch で作品の `url` から動画 `videoId` の作品情報を取る
 * （POST /api/videos/{id}/work-info-fetches）。結果はサーバーが 10 分持ち、動画は変わらない。動画が無ければ
 * 404 `not_found` で、実行の失敗は searchWorkInfo と同じである。
 */
export function fetchWorkInfo(
  videoId: number,
  pluginId: number,
  url: string,
  signal?: AbortSignal,
): Promise<WorkInfoFetch> {
  return request<WorkInfoFetch>(`/api/videos/${String(videoId)}/work-info-fetches`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ pluginId, url }),
    signal,
  });
}

/**
 * applyWorkInfoFetch は取得の結果 `fetchId` のうち `fields` の項目を動画に反映する
 * （POST …/work-info-fetches/{fetchId}/apply）。応答は反映後の動画と付けなかったタグの名前である。結果は
 * 使い切りで、期限切れか取得の後にプラグインが変わっていれば 409 `work_info_fetch_stale` で失敗する。
 * 成功したら一覧の控えを捨て、タグを付けたなら共有のタグの一覧も取り直す。
 */
export async function applyWorkInfoFetch(
  videoId: number,
  fetchId: string,
  fields: readonly WorkInfoField[],
  signal?: AbortSignal,
): Promise<WorkInfoFetchApplied> {
  const applied = await request<WorkInfoFetchApplied>(
    `/api/videos/${String(videoId)}/work-info-fetches/${encodeURIComponent(fetchId)}/apply`,
    { method: "POST", headers: jsonHeaders, body: JSON.stringify({ fields }), signal },
  );
  // 控えは離れた一覧の題名とタグを持ち、一覧が外れている間の `video` 通知では直らないので、
  // 捨てて戻ったときに読み直させる（setVideoDetails と同じ）。タグを付けたなら、作られた
  // タグを候補に出すため共有のタグの一覧も取り直す（attachVideoTagByName と同じ）。
  if (fields.includes("tags")) afterTagsAppliedByName();
  else clearListSnapshot();
  return applied;
}
