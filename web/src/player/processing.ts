import type { Video } from "../api/client";
import { t, type UiText } from "../i18n";

export type StageState = "done" | "active" | "waiting" | "failed";

export interface Stage {
  id: keyof typeof t.player.stages.names;
  name: UiText;
  state: StageState;
}

type GenerationState = Video["probeState"];

/** Jobs run serially: only the first pending stage is active. Failed stages are terminal, and absent seek-preview state remains pending. */
export function processingStages(video: Video): Stage[] {
  const names = t.player.stages.names;
  const generation: [Stage["id"], GenerationState][] = [
    ["probe", video.probeState],
    ["thumbnail", video.thumbnailState],
    ["seekPreview", video.seekThumbnailState ?? "pending"],
    ["preview", video.previewState],
  ];
  let activeTaken = false;
  const stages: Stage[] = [{ id: "detect", name: names.detect, state: "done" }];
  for (const [id, state] of generation) {
    if (state === "pending") {
      stages.push({ id, name: names[id], state: activeTaken ? "waiting" : "active" });
      activeTaken = true;
    } else {
      stages.push({ id, name: names[id], state });
    }
  }
  return stages;
}

export function creatingLine(video: Video): UiText | null {
  if (video.probeState !== "done") return null;
  const names = t.player.creating;
  const creating = (
    [
      [names.thumbnail, video.thumbnailState],
      [names.seekPreview, video.seekThumbnailState],
      [names.preview, video.previewState],
    ] as const
  )
    .filter(([, state]) => state === "pending")
    .map(([name]) => name);
  if (creating.length === 0) return null;
  return names.line(creating);
}
