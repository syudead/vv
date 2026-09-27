import type { Video } from "../api/client";
import { t, type UiText } from "../i18n";

/**
 * 取り込みの段階表示と作成中の 1 行の出し分け（plan の Structural Decisions 11、
 * ui-design「Processing stages」「Creating line」）。
 */

export type StageState = "done" | "active" | "waiting" | "failed";

export interface Stage {
  /** 段を見分ける鍵（描画の key）。 */
  id: keyof typeof t.player.stages.names;
  name: UiText;
  state: StageState;
}

type GenerationState = Video["probeState"];

/**
 * processingStages は 5 つの段を固定の順で返す。
 *
 * - 「ファイルの検出」は常に完了。動画の行があることが、検出が済んだことを意味する。
 * - `pending` の段のうち先頭だけを「処理中」、残りを「待機中」にする。ジョブは 1 本ずつ
 *   順に動くので、動いているのは先頭だけである。
 * - `failed` は「終わった」として「作成できませんでした」にする。
 * - シーク用プレビューの状態は読み取り前の応答に入らない。そのときは未完了として扱う。
 */
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

/**
 * creatingLine は、読み取りが終わっていて作成中のものが残っているときの 1 行を返す。
 * 無ければ null。`failed` だけが残っても null にする。
 */
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
