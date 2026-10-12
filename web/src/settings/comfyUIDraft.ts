import type { ComfyUISettings, UpdateComfyUISettings } from "../api/client";

export type Provider = UpdateComfyUISettings["provider"];
export type Model = ComfyUISettings["model"];

export const providerOrder: readonly Provider[] = ["self_hosted", "cloud"];

/** modelOrder は選べるモデルの並びで、既定のモデルが先頭である。 */
export const modelOrder: readonly Model[] = ["flux2_klein_4b", "qwen_image_edit_2511"];

/** Draft は画面で編集中の値である。`apiKey` は入力欄の値で、保存済みのキーは入らない。 */
export interface Draft {
  provider: Provider;
  url: string;
  apiKey: string;
  model: Model;
}

/** draftOf は保存済みの値から下書きを作る。何も保存されていなければ自前の ComfyUI を選ぶ。 */
export function draftOf(settings: ComfyUISettings): Draft {
  return {
    provider: settings.provider ?? "self_hosted",
    url: settings.url ?? "",
    apiKey: "",
    model: settings.model,
  };
}

/** sameValues は 2 つの下書きの値がすべて同じかを返す。 */
function sameValues(a: Draft, b: Draft): boolean {
  return (
    a.provider === b.provider &&
    a.url === b.url &&
    a.apiKey === b.apiKey &&
    a.model === b.model
  );
}

/**
 * afterSave は保存の応答を受けた後の下書きを返す。送った後に値が変わっていなければ保存後の値に
 * 戻す。変わっていれば送った後の編集を残し、キーの欄は送ったキーのままのときだけ空にする。
 */
export function afterSave(current: Draft, sent: Draft, saved: ComfyUISettings): Draft {
  if (sameValues(current, sent)) return draftOf(saved);
  return current.apiKey === sent.apiKey ? { ...current, apiKey: "" } : current;
}

/**
 * bodyOf は下書きを保存の要求にする。キーの欄が空なら `apiKey` を省き、保存済みのキーを残す。
 * モデルはいつも送る。
 */
export function bodyOf(draft: Draft): UpdateComfyUISettings {
  const body: UpdateComfyUISettings = { provider: draft.provider, model: draft.model };
  if (draft.provider === "self_hosted") body.url = draft.url;
  if (draft.apiKey !== "") body.apiKey = draft.apiKey;
  return body;
}

/**
 * isDirty は下書きに保存していない値があるかを返す。何も保存されていなければいつも真である。
 * 自前の ComfyUI でないときの URL は保存しないので比べない。
 */
export function isDirty(draft: Draft, settings: ComfyUISettings): boolean {
  if (settings.provider === undefined) return true;
  return (
    draft.provider !== settings.provider ||
    (draft.provider === "self_hosted" && draft.url !== (settings.url ?? "")) ||
    draft.apiKey !== "" ||
    draft.model !== settings.model
  );
}
