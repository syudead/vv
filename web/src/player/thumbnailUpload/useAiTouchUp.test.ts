import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ThumbnailEdit } from "../../api/client";
import { POLL_MS, useAiTouchUp } from "./useAiTouchUp";

// The client and decoding are the boundaries; polling runs on virtual time.

const client = vi.hoisted(() => ({
  getThumbnailEditReadiness: vi.fn(),
  startThumbnailEditFromImage: vi.fn(),
  runThumbnailEdit: vi.fn(),
  getThumbnailEdit: vi.fn(),
  getThumbnailEditResult: vi.fn(),
  discardThumbnailEdit: vi.fn(),
}));
vi.mock("../../api/client", async (original) => ({
  ...(await original<typeof import("../../api/client")>()),
  ...client,
}));

const compose = vi.hoisted(() => ({ loadUploadImage: vi.fn() }));
vi.mock("./composeImage", () => compose);

function session(state: ThumbnailEdit["state"], extra: Partial<ThumbnailEdit> = {}) {
  return {
    id: "E1",
    videoId: 7,
    state,
    source: "image",
    frameUrl: "/api/thumbnail-edits/E1/frame",
    frameWidth: 2048,
    frameHeight: 1152,
    ...extra,
  } as ThumbnailEdit;
}

const framing = new Blob(["png"]);
const output = new Blob(["out"]);
const touched = { bitmap: { close: vi.fn() }, width: 2048, height: 1152 };

async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}

async function poll() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(POLL_MS);
  });
}

describe("useAiTouchUp", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    for (const fn of Object.values(client)) fn.mockReset();
    compose.loadUploadImage.mockReset();
    touched.bitmap.close.mockReset();
    client.getThumbnailEditReadiness.mockResolvedValue({ connection: true });
    client.startThumbnailEditFromImage.mockResolvedValue(session("ready"));
    client.runThumbnailEdit.mockResolvedValue(session("queued"));
    client.discardThumbnailEdit.mockResolvedValue(undefined);
    client.getThumbnailEditResult.mockResolvedValue(output);
    compose.loadUploadImage.mockResolvedValue({ image: touched });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("枠の絵から編集を始めて指示で走らせ、終わるまで読み、出力を受け取ったら編集を捨てる", async () => {
    client.getThumbnailEdit
      .mockResolvedValueOnce(session("running", { progress: { value: 3, max: 8 } }))
      .mockResolvedValueOnce(session("succeeded", { resultUrl: "/r" }));
    const { result } = renderHook(() => useAiTouchUp(7));
    await flush();
    expect(result.current.ready).toBe(true);

    act(() => result.current.run(Promise.resolve(framing), "Remove the text"));
    await flush();
    expect(client.startThumbnailEditFromImage).toHaveBeenCalledWith(
      7,
      framing,
      expect.any(AbortSignal),
    );
    expect(client.runThumbnailEdit).toHaveBeenCalledWith(
      "E1",
      { prompt: "Remove the text" },
      expect.any(AbortSignal),
    );
    expect(result.current.state).toMatchObject({
      status: "running",
      session: { state: "queued" },
    });
    await poll();
    expect(result.current.state).toMatchObject({
      status: "running",
      session: { progress: { value: 3, max: 8 } },
    });
    expect(client.discardThumbnailEdit).not.toHaveBeenCalled();
    await poll();
    expect(compose.loadUploadImage).toHaveBeenCalledWith(output);
    expect(result.current.state).toEqual({ status: "result", image: touched });
    expect(client.discardThumbnailEdit).toHaveBeenCalledWith("E1");

    let taken: unknown;
    act(() => {
      taken = result.current.take();
    });
    expect(taken).toBe(touched);
    expect(result.current.state).toEqual({ status: "idle" });
    expect(touched.bitmap.close).not.toHaveBeenCalled();
  });

  it("失敗した実行は理由を出し、捨てた結果と離れたときの結果は閉じる", async () => {
    client.getThumbnailEdit.mockResolvedValueOnce(
      session("failed", { error: { reason: "timed_out" } }),
    );
    const { result, unmount } = renderHook(() => useAiTouchUp(7));
    act(() => result.current.run(Promise.resolve(framing), "x"));
    await flush();
    await poll();
    expect(result.current.state).toEqual({
      status: "failed",
      message: "Couldn't generate the image: No result after 10 minutes",
    });

    client.getThumbnailEdit.mockResolvedValue(session("succeeded", { resultUrl: "/r" }));
    act(() => result.current.run(Promise.resolve(framing), "x"));
    await flush();
    await poll();
    act(() => result.current.dismiss());
    expect(touched.bitmap.close).toHaveBeenCalledTimes(1);
    act(() => result.current.run(Promise.resolve(framing), "x"));
    await flush();
    await poll();
    unmount();
    expect(touched.bitmap.close).toHaveBeenCalledTimes(2);
  });

  it("やめると編集を捨て、その後に届いた応答では何も変えない", async () => {
    client.getThumbnailEdit.mockResolvedValue(session("running"));
    const { result, unmount } = renderHook(() => useAiTouchUp(7));
    act(() => result.current.run(Promise.resolve(framing), "x"));
    await flush();
    act(() => result.current.cancel());
    expect(client.discardThumbnailEdit).toHaveBeenCalledWith("E1");
    expect(result.current.state).toEqual({ status: "cancelled" });
    await poll();
    expect(client.getThumbnailEdit).not.toHaveBeenCalled();
    expect(result.current.state).toEqual({ status: "cancelled" });

    act(() => result.current.run(Promise.resolve(framing), "x"));
    await flush();
    unmount();
    expect(client.discardThumbnailEdit).toHaveBeenCalledTimes(2);
  });

  it("絵を作れない・始められないときは失敗を出し、ComfyUI が無ければ使えない", async () => {
    client.getThumbnailEditReadiness.mockResolvedValue({ connection: false });
    client.startThumbnailEditFromImage.mockRejectedValueOnce(new Error("offline"));
    const { result } = renderHook(() => useAiTouchUp(7));
    await flush();
    expect(result.current.ready).toBe(false);
    act(() => result.current.run(Promise.reject(new Error("toBlob")), "x"));
    await flush();
    expect(result.current.state).toMatchObject({ status: "failed" });
    expect(client.startThumbnailEditFromImage).not.toHaveBeenCalled();
    act(() => result.current.run(Promise.resolve(framing), "x"));
    await flush();
    expect(result.current.state).toMatchObject({ status: "failed" });
  });
});
