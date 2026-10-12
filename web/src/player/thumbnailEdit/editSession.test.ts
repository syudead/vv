import { describe, expect, it } from "vitest";

import type { ThumbnailEdit } from "../../api/client";
import {
  editorReducer,
  type EditorState,
  hasResult,
  initialEditorState,
  needsConfirmation,
} from "./editSession";

const ready: ThumbnailEdit = {
  id: "e1",
  videoId: 7,
  state: "ready",
  source: "frame",
  positionMs: 0,
  frameUrl: "/api/thumbnail-edits/e1/frame",
  frameWidth: 1920,
  frameHeight: 1080,
};

function at(state: EditorState, session: Partial<ThumbnailEdit>): EditorState {
  return editorReducer(state, {
    type: "read",
    session: { ...state.session, ...session },
    at: 1,
  });
}

function started(): EditorState {
  return editorReducer(initialEditorState(ready, 0), {
    type: "started",
    session: { ...ready, state: "queued", elapsedMs: 0 },
    at: 0,
  });
}

const succeeded: Partial<ThumbnailEdit> = {
  state: "succeeded",
  elapsedMs: 9_000,
  resultUrl: "/api/thumbnail-edits/e1/result",
};

describe("editorReducer", () => {
  it("始めたばかりは Compose で、閉じるのに確かめない", () => {
    const state = initialEditorState(ready, 0);
    expect(state.view).toBe("compose");
    expect(hasResult(state)).toBe(false);
    expect(needsConfirmation(state)).toBe(false);
  });

  it("実行を送ると Running になり、成功を読んだら Compare へ移る", () => {
    let state = started();
    expect(state.view).toBe("running");
    expect(state.ran).toBe(true);
    expect(needsConfirmation(state)).toBe(true);
    state = at(state, { state: "preparing", elapsedMs: 1_000 });
    expect(state.view).toBe("running");
    state = at(state, succeeded);
    expect(state.view).toBe("compare");
    expect(hasResult(state)).toBe(true);
    expect(needsConfirmation(state)).toBe(true);
  });

  it("失敗を読んだら理由と提供元の文を持って Compose に戻る", () => {
    const state = at(started(), {
      state: "failed",
      error: { reason: "unreachable", message: "connection refused" },
    });
    expect(state.view).toBe("compose");
    expect(state.failure).toEqual({
      title: "Couldn't generate the image: ComfyUI can't be reached",
      message: "connection refused",
    });
    expect(needsConfirmation(state)).toBe(false);
  });

  it("中止の応答が cancelled なら Compose に戻り、succeeded なら Compare へ移る", () => {
    const cancelled = at(started(), { state: "cancelled" });
    expect(cancelled.view).toBe("compose");
    expect(cancelled.cancelled).toBe(true);
    expect(at(started(), succeeded).view).toBe("compare");
  });

  it("結果から戻って入力を変える間は結果を見直せ、実行を送ったら二度と出さない", () => {
    let state = editorReducer(at(started(), succeeded), { type: "change" });
    expect(state.view).toBe("compose");
    expect(hasResult(state)).toBe(true);
    // 結果を持つ間の 60 秒ごとの読み取りは、見え方を変えない。
    state = at(state, succeeded);
    expect(state.view).toBe("compose");
    expect(editorReducer(state, { type: "showResult" }).view).toBe("compare");

    const rejected = editorReducer(state, {
      type: "rejected",
      failure: { title: "Couldn't generate the image: x" as never },
    });
    expect(hasResult(rejected)).toBe(false);
    expect(editorReducer(rejected, { type: "showResult" }).view).toBe("compose");
  });

  it("セッションが終わったら、その後の読み取りでは変えず、閉じるのに確かめない", () => {
    const ended = editorReducer(started(), { type: "ended" });
    expect(ended.ended).toBe(true);
    expect(needsConfirmation(ended)).toBe(false);
    expect(at(ended, succeeded)).toBe(ended);
  });
});
