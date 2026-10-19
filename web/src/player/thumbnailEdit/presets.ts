export type PresetId = "enhance" | "unblur" | "removeText" | "fillEmpty";

export interface Preset {
  id: PresetId;
  /** Sent to ComfyUI in English, whatever the page's language, as the edit models follow English best. */
  prompt: string;
}

/** Fixed instructions shared by the thumbnail preview and the ComfyUI editor. */
export const PRESETS: readonly Preset[] = [
  {
    id: "enhance",
    prompt:
      "Restore this image in high quality: remove noise and compression artifacts and sharpen fine detail. Keep the composition, colours and content exactly as they are.",
  },
  {
    id: "unblur",
    prompt:
      "Remove the blur so the whole image looks in sharp focus. Keep everything else exactly as it is.",
  },
  {
    id: "removeText",
    prompt:
      "Remove all text, captions, logos and watermarks, and fill those areas naturally to match their surroundings. Keep everything else exactly as it is.",
  },
  {
    id: "fillEmpty",
    prompt:
      "The picture does not cover the whole frame: replace the blurred or black borders by extending the picture naturally, so the frame shows one continuous image. Keep the existing picture exactly as it is.",
  },
];
