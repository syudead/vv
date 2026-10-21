import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  installPersonServer,
  person,
  renderPage,
  sent,
  server,
} from "../../testing/personPage";
import { encodeFaceFrame } from "./faceFrameImage";

vi.mock("./faceFrameImage", () => ({
  encodeFaceFrame: vi.fn(() =>
    Promise.resolve(new Blob(["face"], { type: "image/jpeg" })),
  ),
}));

installPersonServer();

/** jsdom decodes no images, so every frame loads at once as 1000 by 500 px. */
class LoadedImage {
  naturalWidth = 1000;
  naturalHeight = 500;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  set src(_: string) {
    queueMicrotask(() => this.onload?.());
  }
}

beforeEach(() => {
  vi.stubGlobal("Image", LoadedImage);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("FacePickerDialog", () => {
  it("saves a portrait cut from the chosen face's frame, a second later", async () => {
    const user = userEvent.setup();
    server.person = person({ imageRevision: 2 });
    server.appearances = [
      {
        appearanceId: 3,
        videoId: 5,
        title: "Beach day",
        spans: [{ start: 10, end: 15 }],
        hasCrop: true,
        focus: { time: 12.5, x: 0.5, y: 0.2, w: 0.05 },
      },
      { appearanceId: 4, title: "Gone video", spans: [], hasCrop: true },
    ];
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Change face" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Choose from videos…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: /Choose a face/ });

    const [list] = sent("GET", "/api/people/7/appearances");
    expect(list?.url.searchParams.get("sort")).toBe("mostSimilar");
    // A face whose video is gone has no frame to cut, so only one face is offered.
    expect(within(dialog).getAllByRole("radio")).toHaveLength(1);
    expect(within(dialog).getByRole("radio", { name: "Face in Beach day" })).toBeTruthy();
    expect(within(dialog).getByText("At 0:12")).toBeTruthy();

    await user.click(within(dialog).getByRole("button", { name: "1 second later" }));
    expect(within(dialog).getByText("At 0:13")).toBeTruthy();
    await waitFor(() =>
      expect(dialog.querySelector("img[src*='/frame']")?.getAttribute("src")).toBe(
        "/api/videos/5/frame?positionMs=13500",
      ),
    );
    await user.click(within(dialog).getByRole("button", { name: "Set as face" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const [image, rect] = vi.mocked(encodeFaceFrame).mock.calls[0] ?? [];
    expect(image).toBeInstanceOf(LoadedImage);
    expect(rect?.width).toBeCloseTo(110);
    expect((rect?.x ?? 0) + (rect?.width ?? 0) / 2).toBeCloseTo(500);
    const [upload] = sent("PUT", "/api/people/7/image");
    expect((upload?.body as FormData).get("image")).toBeInstanceOf(Blob);
    expect(screen.getByRole("img", { name: "Aoi" }).getAttribute("src")).toBe(
      "/api/people/7/image?v=3",
    );
  });
});
