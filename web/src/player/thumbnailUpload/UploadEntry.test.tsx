import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../../api/client";
import { WHOLE_IMAGE } from "./framing";
import { useThumbnailUpload } from "./UploadEntry";

// Cover upload failures and navigation; VideoPage.thumbnailUpload.test.tsx covers successful application.

const compose = vi.hoisted(() => ({
  loadUploadImage: vi.fn(),
  encodeComposition: vi.fn(),
}));
vi.mock("./composeImage", () => compose);

const client = vi.hoisted(() => ({ uploadThumbnailImage: vi.fn() }));
vi.mock("../../api/client", async (original) => ({
  ...(await original<typeof import("../../api/client")>()),
  uploadThumbnailImage: client.uploadThumbnailImage,
}));

const video = {
  id: 7,
  width: 1920,
  height: 1080,
  contentVersion: "abc",
} as Video;

function png() {
  return new File([new Uint8Array([0x89, 0x50])], "photo.png", { type: "image/png" });
}

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function setup(initial: Video = video) {
  const report = vi.fn();
  const onChanged = vi.fn();
  const onStale = vi.fn();
  const hook = renderHook(
    ({ current }: { current: Video }) =>
      useThumbnailUpload(current, report, onChanged, onStale),
    { initialProps: { current: initial } },
  );
  return { ...hook, report, onChanged, onStale };
}

describe("useThumbnailUpload", () => {
  const bitmap = { close: vi.fn() };

  beforeEach(() => {
    bitmap.close.mockReset();
    compose.loadUploadImage.mockReset();
    compose.loadUploadImage.mockResolvedValue({
      image: { bitmap, width: 10, height: 20 },
    });
    compose.encodeComposition.mockReset();
    client.uploadThumbnailImage.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("プレビューを開いたときの動画の比率と内容の版を持ち、読む間は preparing である", async () => {
    const { result } = setup({ ...video, displayAspectRatio: 2 });
    act(() => result.current.choose(png()));
    expect(result.current.preparing).toBe(true);
    await waitFor(() => expect(result.current.preview).not.toBeNull());
    expect(result.current.preparing).toBe(false);
    expect(result.current.preview).toMatchObject({
      name: "photo.png",
      aspect: 2,
      contentVersion: "abc",
    });
    act(() => result.current.choose(png()));
    expect(compose.loadUploadImage).toHaveBeenCalledTimes(1);
  });

  it("読み込みそのものが失敗すれば、読めない画像の理由で断り、また選べる", async () => {
    compose.loadUploadImage.mockRejectedValueOnce(new Error("io"));
    const { result, report } = setup();
    act(() => result.current.choose(png()));
    await waitFor(() =>
      expect(report).toHaveBeenLastCalledWith(
        "Couldn't upload the image: The file isn't a PNG, JPEG or still WebP, or it can't be read.",
      ),
    );
    expect(result.current.preview).toBeNull();
    act(() => result.current.choose(png()));
    expect(compose.loadUploadImage).toHaveBeenCalledTimes(2);
  });

  it("読む間に別の動画へ移れば、読んだ画像を閉じてプレビューを開かない", async () => {
    const loading = deferred<unknown>();
    compose.loadUploadImage.mockReturnValueOnce(loading.promise);
    const { result, rerender } = setup();
    act(() => result.current.choose(png()));
    rerender({ current: { ...video, id: 8 } });
    await act(async () => {
      loading.resolve({ image: { bitmap, width: 10, height: 20 } });
      await loading.promise;
    });
    expect(bitmap.close).toHaveBeenCalled();
    expect(result.current.preview).toBeNull();
    act(() => result.current.choose(png()));
    expect(compose.loadUploadImage).toHaveBeenCalledTimes(2);
  });

  it("読む間に別の動画へ移って断られても、何も出さずにまた選べる", async () => {
    const loading = deferred<unknown>();
    compose.loadUploadImage.mockReturnValueOnce(loading.promise);
    const { result, rerender, report } = setup();
    act(() => result.current.choose(png()));
    rerender({ current: { ...video, id: 8 } });
    await act(async () => {
      loading.resolve({ refusal: "thumbnail_upload_invalid" });
      await loading.promise;
    });
    expect(report).toHaveBeenLastCalledWith(null);
    act(() => result.current.choose(png()));
    expect(compose.loadUploadImage).toHaveBeenCalledTimes(2);
  });

  it("プレビュー中に別の動画へ移ると、何も送らずに閉じる", async () => {
    const { result, rerender } = setup();
    act(() => result.current.choose(png()));
    await waitFor(() => expect(result.current.preview).not.toBeNull());
    rerender({ current: { ...video, id: 8 } });
    expect(result.current.preview).toBeNull();
    expect(bitmap.close).toHaveBeenCalled();
    expect(client.uploadThumbnailImage).not.toHaveBeenCalled();
    act(() => result.current.choose(png()));
    expect(compose.loadUploadImage).toHaveBeenCalledTimes(2);
  });

  it("送る絵を作れなければ、閉じて失敗を出し、何も送らない", async () => {
    compose.encodeComposition.mockRejectedValueOnce(new Error("toBlob"));
    const { result, report } = setup();
    act(() => result.current.choose(png()));
    await waitFor(() => expect(result.current.preview).not.toBeNull());
    act(() => result.current.apply(WHOLE_IMAGE));
    expect(result.current.applying).toBe(true);
    act(() => result.current.apply(WHOLE_IMAGE));
    act(() => result.current.cancel());
    await waitFor(() => expect(result.current.preview).toBeNull());
    expect(result.current.applying).toBe(false);
    expect(report).toHaveBeenLastCalledWith(
      "Couldn't upload the image: Something went wrong.",
    );
    expect(compose.encodeComposition).toHaveBeenCalledTimes(1);
    expect(client.uploadThumbnailImage).not.toHaveBeenCalled();
  });

  it("プレビューが無いときの適用とやめるは何もしない", () => {
    const { result } = setup();
    act(() => result.current.apply(WHOLE_IMAGE));
    act(() => result.current.cancel());
    expect(compose.encodeComposition).not.toHaveBeenCalled();
  });

  it("内容の版が無い動画では空の版を送り、サーバーが断る", async () => {
    compose.encodeComposition.mockResolvedValueOnce(new Blob(["png"]));
    client.uploadThumbnailImage.mockResolvedValueOnce({ ...video });
    const { result, onChanged } = setup({ ...video, contentVersion: undefined });
    act(() => result.current.choose(png()));
    await waitFor(() => expect(result.current.preview).not.toBeNull());
    act(() => result.current.apply(WHOLE_IMAGE));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    expect(client.uploadThumbnailImage).toHaveBeenCalledWith(7, expect.any(File), "");
  });

  it("ページを離れた後に読めても、送れても、何も反映しない", async () => {
    const loading = deferred<unknown>();
    compose.loadUploadImage.mockReturnValueOnce(loading.promise);
    const { result, unmount, report } = setup();
    act(() => result.current.choose(png()));
    unmount();
    await act(async () => {
      loading.resolve({ image: { bitmap, width: 10, height: 20 } });
      await loading.promise;
    });
    expect(bitmap.close).toHaveBeenCalled();
    expect(report).toHaveBeenCalledTimes(1);
  });
});
