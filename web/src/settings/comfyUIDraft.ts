import type { ComfyUISettings, UpdateComfyUISettings } from "../api/client";

export type Provider = UpdateComfyUISettings["provider"];
export type Model = ComfyUISettings["model"];

export const providerOrder: readonly Provider[] = ["self_hosted", "cloud"];

export const modelOrder: readonly Model[] = ["flux2_klein_4b", "qwen_image_edit_2511"];

/** apiKey contains only newly entered text, never the stored secret. */
export interface Draft {
  provider: Provider;
  url: string;
  apiKey: string;
  model: Model;
}

export function draftOf(settings: ComfyUISettings): Draft {
  return {
    provider: settings.provider ?? "self_hosted",
    url: settings.url ?? "",
    apiKey: "",
    model: settings.model,
  };
}

function sameValues(a: Draft, b: Draft): boolean {
  return (
    a.provider === b.provider &&
    a.url === b.url &&
    a.apiKey === b.apiKey &&
    a.model === b.model
  );
}

/** Preserve edits made after submission; clear the key only if it still equals the submitted key. */
export function afterSave(current: Draft, sent: Draft, saved: ComfyUISettings): Draft {
  if (sameValues(current, sent)) return draftOf(saved);
  return current.apiKey === sent.apiKey ? { ...current, apiKey: "" } : current;
}

/** Omit empty apiKey to retain the stored key; always send model. */
export function bodyOf(draft: Draft): UpdateComfyUISettings {
  const body: UpdateComfyUISettings = { provider: draft.provider, model: draft.model };
  if (draft.provider === "self_hosted") body.url = draft.url;
  if (draft.apiKey !== "") body.apiKey = draft.apiKey;
  return body;
}

/** Unsaved settings are always dirty; ignore URL outside self-hosted mode because it is not persisted. */
export function isDirty(draft: Draft, settings: ComfyUISettings): boolean {
  if (settings.provider === undefined) return true;
  return (
    draft.provider !== settings.provider ||
    (draft.provider === "self_hosted" && draft.url !== (settings.url ?? "")) ||
    draft.apiKey !== "" ||
    draft.model !== settings.model
  );
}
