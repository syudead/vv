import { screen, waitFor, within } from "@testing-library/react";
import { useEffect, useState } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  appliedResult,
  fetchById,
  fetchResult,
  heading,
  json,
  server,
  useFetchServer,
  video,
} from "../testing/workInfoFetchPage";
import type { PlayerControls } from "./playerControls";

// jsdom lacks canvas rendering and image decoding; replace those boundaries.
const compose = vi.hoisted(() => ({
  loadUploadImage: vi.fn(),
  drawComposition: vi.fn(),
  encodeComposition: vi.fn(),
}));

vi.mock("./thumbnailUpload/composeImage", () => compose);

vi.mock("./VideoPlayer", () => ({
  default: function VideoPlayerMock(props: {
    onControls: (controls: PlayerControls | null) => void;
  }) {
    const [initial] = useState(props);
    useEffect(() => () => initial.onControls(null), [initial]);
    return <div data-testid="video-player" />;
  },
  canStartPlayback: () => true,
  initialPlayerStatus: {
    loading: false,
    reconnecting: false,
    playing: false,
    userActive: true,
    ended: false,
    stalled: false,
    positioned: false,
  },
}));

const cover = { bitmap: { close: vi.fn() }, width: 800, height: 450 };
const composed = new Blob(["png"], { type: "image/png" });
const titled = { ...video, title: "取得した題名", contentVersion: "v1" };

function withCover() {
  server.fetch.mockImplementation(() =>
    json(fetchResult({ title: "取得した題名", cover: { width: 800, height: 450 } }), 201),
  );
}

describe("VideoPage work-info fetch cover", () => {
  useFetchServer();

  it("ページで読めないカバーはサーバーが合わせ、使えなければ結果の段で知らせる", async () => {
    withCover();
    compose.loadUploadImage.mockResolvedValue({ refusal: "thumbnail_upload_invalid" });
    server.apply.mockImplementation(() =>
      json(
        appliedResult(titled, {
          skipped: [
            { field: "cover", value: "https://works.example/c.jpg", reason: "invalid" },
          ],
        }),
      ),
    );
    const { user, dialog } = await fetchById();
    const check = within(dialog).getByRole("checkbox", { name: "Cover" });
    expect(check).toHaveProperty("ariaChecked", "true");
    const fetched = within(dialog).getByRole("img", { name: /Fetched cover, 800 × 450/ });
    expect(fetched.getAttribute("src")).toBe("/api/videos/7/work-info-fetches/F1/cover");
    expect(
      within(dialog).getByRole("img", { name: "Current thumbnail" }).getAttribute("src"),
    ).toBe("/api/videos/7/thumbnail");
    await waitFor(() => expect(compose.loadUploadImage).toHaveBeenCalled());
    expect(within(dialog).queryByRole("button", { name: "Adjust framing" })).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({ fields: ["title", "cover"] });
    await within(dialog).findByText("Applied, but some values were left out");
    expect(
      within(dialog)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Cover: https://works.example/c.jpg — couldn't be used as the thumbnail"]);
    expect(server.upload).not.toHaveBeenCalled();
  });

  it("サムネイルの無い動画では、カバー画像の行に取得した画像だけを出す", async () => {
    withCover();
    compose.loadUploadImage.mockResolvedValue({ refusal: "thumbnail_upload_invalid" });
    server.video = { ...video, thumbnailUrl: undefined, thumbnailState: "pending" };
    const { dialog } = await fetchById();
    expect(within(dialog).getByRole("img", { name: /Fetched cover/ })).toBeDefined();
    expect(within(dialog).queryByRole("img", { name: "Current thumbnail" })).toBeNull();
  });

  it("位置を合わせたカバーは、他の項目を反映した後にその構図でアップロードする", async () => {
    withCover();
    compose.loadUploadImage.mockResolvedValue({ image: cover });
    compose.encodeComposition.mockResolvedValue(composed);
    server.apply.mockImplementation(() => json(appliedResult(titled)));
    server.upload.mockImplementation(() =>
      json({ ...titled, thumbnailUrl: "/api/videos/7/thumbnail?v=2" }),
    );
    const { user, dialog } = await fetchById();
    await user.click(
      await within(dialog).findByRole("button", { name: "Adjust framing" }),
    );
    const preview = await screen.findByRole("dialog", { name: "Preview thumbnail" });
    await user.click(within(preview).getByRole("radio", { name: "Fill frame" }));
    await user.click(within(preview).getByRole("button", { name: "Use this framing" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Preview thumbnail" })).toBeNull(),
    );
    expect(server.upload).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(server.apply).toHaveBeenCalledWith({ fields: ["title"] });
    expect(compose.encodeComposition).toHaveBeenCalledWith(cover, 1920 / 1080, {
      mode: "cover",
      zoom: 1,
      panX: 0,
      panY: 0,
    });
    const form = server.upload.mock.calls[0]![0] as FormData;
    expect(form.get("contentVersion")).toBe("v1");
    expect((form.get("image") as File).type).toBe("image/png");
    expect(
      server.requests.filter((r) => r.startsWith("POST")).map((r) => r.split(" ")[1]),
    ).toEqual([
      "/api/videos/7/work-info-fetches",
      "/api/videos/7/work-info-fetches/F1/apply",
      "/api/videos/7/thumbnail-image",
    ]);
    expect(heading().textContent).toBe("取得した題名");
  });

  it("カバーだけを選ぶと結果を反映せずにアップロードし、失敗は結果の段で知らせる", async () => {
    withCover();
    compose.loadUploadImage.mockResolvedValue({ image: cover });
    compose.encodeComposition.mockResolvedValue(composed);
    server.video = { ...video, contentVersion: "v0" };
    server.upload.mockImplementation(() =>
      json({ code: "content_changed", message: "changed" }, 409),
    );
    const { user, dialog } = await fetchById();
    await within(dialog).findByRole("button", { name: "Adjust framing" });
    await user.click(within(dialog).getByRole("checkbox", { name: "Title" }));
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    await within(dialog).findByText("Applied, but some values were left out");
    expect(server.apply).not.toHaveBeenCalled();
    expect((server.upload.mock.calls[0]![0] as FormData).get("contentVersion")).toBe(
      "v0",
    );
    expect(
      within(dialog)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Cover: 800 × 450 — couldn't be used as the thumbnail"]);
  });
});
