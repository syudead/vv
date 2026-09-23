import type { Video } from "../api/client";

/**
 * 取り込みの段階表示と作成中の 1 行の出し分け（plan の Structural Decisions 11、
 * ui-design「Processing stages」「Creating line」）。
 */

export type StageState = "done" | "active" | "waiting" | "failed";

export interface Stage {
  name: string;
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
  const generation: [string, GenerationState][] = [
    ["動画情報の読み取り", video.probeState],
    ["サムネイル", video.thumbnailState],
    ["シーク用プレビュー", video.seekThumbnailState ?? "pending"],
    ["一覧用プレビュー", video.previewState],
  ];
  let activeTaken = false;
  const stages: Stage[] = [{ name: "ファイルの検出", state: "done" }];
  for (const [name, state] of generation) {
    if (state === "pending") {
      stages.push({ name, state: activeTaken ? "waiting" : "active" });
      activeTaken = true;
    } else {
      stages.push({ name, state });
    }
  }
  return stages;
}

/**
 * creatingLine は、読み取りが終わっていて作成中のものが残っているときの 1 行を返す。
 * 無ければ null。`failed` だけが残っても null にする。
 */
export function creatingLine(video: Video): string | null {
  if (video.probeState !== "done") return null;
  const creating = [
    ["サムネイル", video.thumbnailState],
    ["シーク用プレビュー", video.seekThumbnailState],
    ["一覧用プレビュー", video.previewState],
  ]
    .filter(([, state]) => state === "pending")
    .map(([name]) => name);
  if (creating.length === 0) return null;
  return `${creating.join("と")}を作成中 · 再生はできます`;
}
