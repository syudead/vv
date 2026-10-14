import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// 作品情報プラグインの画面 API（specs/049-work-info-plugins/contracts/screen-api.md）。型は
// api/openapi.yaml からの生成物を使う。どの経路も所有者だけである。プラグインの変化はイベントで
// 流れないので、画面は操作のあとに listWorkInfoPlugins で読み直す。

export type WorkInfoPlugin = components["schemas"]["WorkInfoPlugin"];
export type WorkInfoPluginSetting = components["schemas"]["WorkInfoPluginSetting"];
export type WorkInfoPluginState = components["schemas"]["WorkInfoPluginState"];
export type WorkInfoPluginProblem = components["schemas"]["WorkInfoPluginProblem"];
export type WorkInfoPluginList = components["schemas"]["WorkInfoPluginList"];

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
