import { describe, expect, it } from "vitest";

import type { ComfyUISettings } from "../api/client";
import { afterSave, bodyOf, type Draft, draftOf, isDirty } from "./comfyUIDraft";

const saved: ComfyUISettings = {
  provider: "self_hosted",
  url: "http://gpu:8188",
  apiKeySet: false,
  model: "flux2_klein_4b",
};

describe("isDirty", () => {
  it("is false for the saved values and true when any saved value changes", () => {
    const draft = draftOf(saved);
    expect(isDirty(draft, saved)).toBe(false);
    expect(isDirty({ ...draft, model: "qwen_image_edit_2511" }, saved)).toBe(true);
    expect(isDirty({ ...draft, url: "http://other:8188" }, saved)).toBe(true);
    expect(isDirty({ ...draft, apiKey: "k" }, saved)).toBe(true);
    expect(isDirty({ ...draft, provider: "cloud" }, saved)).toBe(true);
  });

  it("is always true when nothing is saved", () => {
    const empty: ComfyUISettings = { apiKeySet: false, model: "flux2_klein_4b" };
    expect(isDirty(draftOf(empty), empty)).toBe(true);
  });
});

describe("bodyOf", () => {
  it("always sends the model, and the URL only for My own ComfyUI", () => {
    const draft: Draft = {
      provider: "cloud",
      url: "http://gpu:8188",
      apiKey: "",
      model: "qwen_image_edit_2511",
    };
    expect(bodyOf(draft)).toEqual({ provider: "cloud", model: "qwen_image_edit_2511" });
  });
});

describe("afterSave", () => {
  it("keeps a model chosen while the save was on its way", () => {
    const sent = draftOf(saved);
    const current: Draft = { ...sent, model: "qwen_image_edit_2511" };
    expect(afterSave(current, sent, saved).model).toBe("qwen_image_edit_2511");
    expect(afterSave(sent, sent, { ...saved, model: "qwen_image_edit_2511" }).model).toBe(
      "qwen_image_edit_2511",
    );
  });
});
